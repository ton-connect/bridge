package storagev3

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sethvargo/go-retry"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal/models"
)

type ValkeyStorage struct {
	client      redis.UniversalClient
	pubSubConn  *redis.PubSub
	subscribers map[string][]chan<- models.SseMessage
	subMutex    sync.RWMutex
}

// NewValkeyStorage creates a new Valkey storage instance
// Supports both single node and cluster modes
// For cluster mode, discovers cluster topology using CLUSTER SLOTS command
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	log := log.WithField("prefix", "NewValkeyStorage")

	var client redis.UniversalClient

	// Parse the primary URI
	opts, err := redis.ParseURL(strings.TrimSpace(valkeyURI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// First, connect to the single node to check if it's part of a cluster
	tempClient := redis.NewClient(opts)
	ctxTemp, cancelTemp := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelTemp()

	// Try to get cluster info with retry logic
	clusterSlots, err := retry.DoValue(ctxTemp, retry.WithMaxRetries(7, retry.NewFibonacci(500*time.Millisecond)),
		func(ctx context.Context) ([]redis.ClusterSlot, error) {
			slots, err := tempClient.ClusterSlots(ctx).Result()
			if err != nil {
				// TODO which errors should we filter?
				log.Debugf("retrying cluster slots query due to: %v", err)
				return nil, retry.RetryableError(err)
			}
			return slots, nil
		})

	if err != nil {
		log.Warnf("failed to get cluster slots after retries: %v", err)
		clusterSlots = []redis.ClusterSlot{} // Fallback to single-node mode
	}

	if err := tempClient.Close(); err != nil {
		log.Warnf("failed to close temporary client: %v", err)
	}

	log.Infof("cluster slots result: %+v", clusterSlots)

	if len(clusterSlots) == 0 {
		// Not a cluster or cluster command failed, use single-node mode
		log.Info("Using single-node mode")
		client = redis.NewClient(opts)
	} else {
		// Extract all node addresses from cluster slots
		nodeAddrs := make(map[string]bool)
		for _, slot := range clusterSlots {
			for _, node := range slot.Nodes {
				nodeAddrs[node.Addr] = true
			}
		}

		// Convert to slice
		addrs := make([]string, 0, len(nodeAddrs))
		for addr := range nodeAddrs {
			addrs = append(addrs, addr)
		}

		log.Infof("Using cluster mode with %d nodes discovered from CLUSTER SLOTS", len(addrs))
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:     addrs,
			Password:  opts.Password,
			Username:  opts.Username,
			TLSConfig: opts.TLSConfig,
			// Enable automatic cluster redirection handling
			ReadOnly:       false,
			RouteByLatency: true,
			RouteRandomly:  false,
			// Set maximum redirects to handle MOVED responses
			MaxRedirects: 3,
			// Set appropriate timeouts for managed clusters like AWS ElastiCache
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			DialTimeout:  10 * time.Second,
			PoolTimeout:  30 * time.Second,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	log.Info("Successfully connected to Valkey")
	return &ValkeyStorage{
		client:      client,
		subscribers: make(map[string][]chan<- models.SseMessage),
	}, nil
}

// Pub publishes a message to Redis and stores it with TTL
func (s *ValkeyStorage) Pub(ctx context.Context, message models.SseMessage, ttl int64) error {
	log := log.WithField("prefix", "ValkeyStorage.Pub")

	// Publish to Redis channel
	channel := fmt.Sprintf("client:%s", message.To)
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = s.client.Publish(ctx, channel, messageData).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message to channel %s: %w", channel, err)
	}

	// Store message with TTL as backup for offline clients
	expireTime := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	err = s.client.ZAdd(ctx, channel, redis.Z{
		Score:  float64(expireTime),
		Member: messageData,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to store message in sorted set for channel %s: %w", channel, err)
	}

	// Set expiration on the key itself
	s.client.Expire(ctx, channel, time.Duration(ttl+60)*time.Second) // TODO remove 60 seconds buffer?

	log.Debugf("published and stored message for client %s with TTL %d seconds", message.To, ttl)
	return nil
}

// Sub subscribes to Redis channels for the given keys and sends historical messages after lastEventId
func (s *ValkeyStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	log := log.WithField("prefix", "ValkeyStorage.Sub")

	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	// Add messageCh to subscribers for each key
	for _, key := range keys {
		if s.subscribers[key] == nil {
			s.subscribers[key] = make([]chan<- models.SseMessage, 0)
		}
		s.subscribers[key] = append(s.subscribers[key], messageCh)
	}

	// Send historical messages for each key
	now := time.Now().Unix()
	for _, key := range keys {
		clientKey := fmt.Sprintf("client:%s", key)

		// Remove expired messages first
		// TODO support expired messages but not delivered log
		s.client.ZRemRangeByScore(ctx, clientKey, "0", fmt.Sprintf("%d", now))

		// Get all remaining messages
		messages, err := s.client.ZRange(ctx, clientKey, 0, -1).Result()
		if err != nil {
			if err != redis.Nil {
				log.Errorf("failed to get historical messages for client %s: %v", key, err)
			}
			continue // No messages for this client or error occurred
		}

		// Parse and send historical messages
		for _, msgData := range messages {
			var msg models.SseMessage
			err := json.Unmarshal([]byte(msgData), &msg)
			if err != nil {
				log.Errorf("failed to unmarshal historical message: %v", err)
				continue
			}

			// Filter by event ID - only send messages after lastEventId
			if msg.EventId > lastEventId {
				select {
				case messageCh <- msg:
				default:
					// Channel is full or closed, skip
				}
			}
		}
	}

	// Create channels list for subscription
	channels := make([]string, len(keys))
	for i, key := range keys {
		channels[i] = fmt.Sprintf("client:%s", key)
	}

	// If this is the first subscription, start the pub-sub connection
	if s.pubSubConn == nil {
		s.pubSubConn = s.client.Subscribe(ctx, channels...)
		go s.handlePubSub()
	} else {
		// Subscribe to additional channels
		err := s.pubSubConn.Subscribe(ctx, channels...)
		if err != nil {
			log.Errorf("failed to subscribe to additional channels: %v", err)
		}
	}

	log.Debugf("subscribed to channels for keys: %v", keys)
	return nil
}

// Unsub unsubscribes from Redis channels for the given keys
func (s *ValkeyStorage) Unsub(ctx context.Context, keys []string, messageCh chan<- models.SseMessage) error {
	log := log.WithField("prefix", "ValkeyStorage.Unsub")

	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	channelsToUnsub := make([]string, 0)

	for _, key := range keys {
		subscribers, exists := s.subscribers[key]
		if !exists {
			continue
		}

		// Remove only the specific messageCh from subscribers
		newSubscribers := make([]chan<- models.SseMessage, 0, len(subscribers))
		for _, ch := range subscribers {
			if ch != messageCh {
				newSubscribers = append(newSubscribers, ch)
			}
		}

		if len(newSubscribers) == 0 {
			// No more subscribers for this key, clean up
			delete(s.subscribers, key)
			channel := fmt.Sprintf("client:%s", key)
			channelsToUnsub = append(channelsToUnsub, channel)
		} else {
			// Still have subscribers, just update the list
			s.subscribers[key] = newSubscribers
		}
	}

	// Only unsubscribe from Redis channels that have NO subscribers left
	if s.pubSubConn != nil && len(channelsToUnsub) > 0 {
		err := s.pubSubConn.Unsubscribe(ctx, channelsToUnsub...)
		if err != nil {
			return fmt.Errorf("failed to unsubscribe from channels: %w", err)
		}
	}

	log.Debugf("unsubscribed messageCh from keys: %v (redis channels unsubbed: %v)", keys, channelsToUnsub)
	return nil
}

// handlePubSub processes incoming Redis pub-sub messages
func (s *ValkeyStorage) handlePubSub() {
	log := log.WithField("prefix", "ValkeyStorage.handlePubSub")

	for msg := range s.pubSubConn.Channel() {
		// Parse channel name to get client key
		var key string
		if len(msg.Channel) > 7 && msg.Channel[:7] == "client:" {
			key = msg.Channel[7:]
		} else {
			continue
		}

		// Parse message
		var sseMessage models.SseMessage
		err := json.Unmarshal([]byte(msg.Payload), &sseMessage)
		if err != nil {
			log.Errorf("failed to unmarshal pub-sub message: %v", err)
			continue
		}

		// Send to all subscribers for this key
		s.subMutex.RLock()
		subscribers, exists := s.subscribers[key]
		if exists {
			for _, ch := range subscribers {
				select {
				case ch <- sseMessage:
				default:
					// Channel is full or closed, skip
				}
			}
		}
		s.subMutex.RUnlock()
	}
}

// AddConnection stores connection info in Valkey with TTL
// Key pattern: conn:full:{clientID}:{ip}:{origin}
func (s *ValkeyStorage) AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error {
	log := log.WithField("prefix", "ValkeyStorage.AddConnection")

	key := fmt.Sprintf("conn:full:%s:%s:%s", conn.ClientID, conn.IP, conn.Origin)

	data := map[string]interface{}{
		"client_id":  conn.ClientID,
		"user_agent": conn.UserAgent,
		"created_at": time.Now().Unix(),
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, data)
	pipe.Expire(ctx, key, ttl)

	// Also add to clientID index for efficient lookup
	indexKey := fmt.Sprintf("conn:idx:%s", conn.ClientID)
	pipe.SAdd(ctx, indexKey, key)
	pipe.Expire(ctx, indexKey, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store connection: %w", err)
	}

	log.Debugf("stored connection for client %s from %s", conn.ClientID, conn.IP)
	return nil
}

// VerifyConnection checks if connection matches cached data
// Returns: "ok" (exact match), "warning" (same origin different IP), "danger" (different origin), or "unknown" (no cached data)
func (s *ValkeyStorage) VerifyConnection(ctx context.Context, conn ConnectionInfo) (string, error) {
	log := log.WithField("prefix", "ValkeyStorage.VerifyConnection")

	// Check for exact match first
	exactKey := fmt.Sprintf("conn:full:%s:%s:%s", conn.ClientID, conn.IP, conn.Origin)
	exists, err := s.client.Exists(ctx, exactKey).Result()
	if err != nil {
		return "", fmt.Errorf("failed to check connection existence: %w", err)
	}
	if exists > 0 {
		log.Debugf("connection verified OK for client %s", conn.ClientID)
		return "ok", nil
	}

	// Get all connections for this clientID
	indexKey := fmt.Sprintf("conn:idx:%s", conn.ClientID)
	keys, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		if err == redis.Nil {
			log.Debugf("no cached connections for client %s", conn.ClientID)
			return "unknown", nil
		}
		return "", fmt.Errorf("failed to get connection index: %w", err)
	}

	if len(keys) == 0 {
		log.Debugf("no cached connections for client %s", conn.ClientID)
		return "unknown", nil
	}

	// Check for partial matches
	leastSuspicious := "danger"
	for _, key := range keys {
		// Extract origin from key: conn:full:{clientID}:{ip}:{origin}
		parts := strings.Split(key, ":")
		if len(parts) < 5 {
			continue
		}

		cachedOrigin := strings.Join(parts[4:], ":") // Handle origins with colons

		if cachedOrigin == conn.Origin {
			leastSuspicious = "warning"
		}
	}

	log.Debugf("connection verification result: %s for client %s", leastSuspicious, conn.ClientID)
	return leastSuspicious, nil
}

// HealthCheck verifies the connection to Valkey
func (s *ValkeyStorage) HealthCheck() error {
	log := log.WithField("prefix", "ValkeyStorage.HealthCheck")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("valkey health check failed: %w", err)
	}

	log.Info("Valkey is healthy")
	return nil
}
