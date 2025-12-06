package storagev3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/models"
)

type ValkeyStorage struct {
	client        redis.UniversalClient
	shardedPubSub *ShardedPubSubManager // valkey-go based sharded pub/sub
	subscribers   map[string][]chan<- models.SseMessage
	subMutex      sync.RWMutex
}

// NewValkeyStorage creates a Valkey-backed storage client.
// Expects a Redis cluster URL (parsed by redis.ParseURL) and requires
// Redis Cluster + Redis 7+ sharded pub/sub. Returns *ValkeyStorage or error.
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	log := log.WithField("prefix", "NewValkeyStorage")

	opts, err := redis.ParseURL(strings.TrimSpace(valkeyURI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	if err := detectClusterMode(opts); err != nil {
		return nil, fmt.Errorf("failed to detect cluster mode or redis endpoint is not in cluster mode: %w", err)
	}

	clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:          []string{opts.Addr},
		Username:       opts.Username,
		Password:       opts.Password,
		TLSConfig:      opts.TLSConfig,
		ReadOnly:       false,
		RouteByLatency: true,
		MaxRedirects:   3,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		DialTimeout:    10 * time.Second,
		PoolTimeout:    30 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := clusterClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	logDiscoveredNodes(ctx, clusterClient)

	if !supportsShardedPubSub(ctx, clusterClient) {
		return nil, fmt.Errorf("redis server does not support sharded pub/sub; requires redis >= 7.0")
	}

	log.Info("successfully connected to Valkey/Redis")

	// Create sharded pub/sub manager using valkey-go
	// This solves the go-redis issue where SSUBSCRIBE only works with channels on the same shard
	shardedPubSub, err := NewShardedPubSubManager(
		[]string{opts.Addr},
		opts.Username,
		opts.Password,
	)
	if err != nil {
		if closeErr := clusterClient.Close(); closeErr != nil {
			log.WithError(closeErr).Error("failed to close cluster client")
		}
		return nil, fmt.Errorf("failed to create sharded pub/sub manager: %w", err)
	}

	log.Info("successfully created sharded pub/sub manager with valkey-go")

	return &ValkeyStorage{
		client:        clusterClient,
		shardedPubSub: shardedPubSub,
		subscribers:   make(map[string][]chan<- models.SseMessage),
	}, nil
}

// detectClusterMode checks if the Redis endpoint is in cluster mode
func detectClusterMode(opts *redis.Options) error {
	client := redis.NewClient(opts)
	defer func() {
		if err := client.Close(); err != nil {
			log.WithField("prefix", "detectClusterMode").Warnf("failed to close temp redis client: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.ClusterInfo(ctx).Result()
	return err
}

// supportsShardedPubSub checks if the Redis server supports sharded pub/sub (Redis 7+)
func supportsShardedPubSub(ctx context.Context, client redis.UniversalClient) bool {
	cmd := client.Do(ctx, "COMMAND", "INFO", "SPUBLISH")
	if err := cmd.Err(); err != nil {
		return false
	}

	res, err := cmd.Slice()
	if err != nil {
		return false
	}
	return len(res) > 0
}

// logDiscoveredNodes logs all master nodes discovered by go-redis (for debugging/monitoring)
func logDiscoveredNodes(ctx context.Context, client *redis.ClusterClient) {
	log := log.WithField("prefix", "ValkeyStorage.logDiscoveredNodes")

	err := client.ForEachMaster(ctx, func(ctx context.Context, c *redis.Client) error {
		opts := c.Options()
		log.Infof("Discovered master node: %s", opts.Addr)
		return nil
	})

	if err != nil {
		log.Warnf("Failed to enumerate cluster nodes: %v", err)
	}
}

// Pub publishes a message to Redis and stores it with TTL
func (s *ValkeyStorage) Pub(ctx context.Context, message models.SseMessage, ttl int64) error {
	log := log.WithField("prefix", "ValkeyStorage.Pub")

	// Publish to Redis channel using valkey-go's sharded pub/sub
	// Use hash tag client to ensure all channels route to the same shard
	channel := fmt.Sprintf("client:%s", message.To)
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Use valkey-go's SPUBLISH which properly handles sharded pub/sub
	err = s.shardedPubSub.Publish(ctx, channel, messageData)
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

	// Subscribe to channels using valkey-go sharded pub/sub manager
	// This handles channels across different shards correctly
	for _, key := range keys {
		channel := fmt.Sprintf("client:%s", key)
		if err := s.shardedPubSub.Subscribe(ctx, channel, messageCh); err != nil {
			log.Errorf("failed to subscribe to channel %s: %v", channel, err)
			return fmt.Errorf("failed to subscribe to channel %s: %w", channel, err)
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

			// Unsubscribe from the channel using valkey-go manager
			channel := fmt.Sprintf("client:%s", key)
			if err := s.shardedPubSub.Unsubscribe(channel, messageCh); err != nil {
				log.Errorf("failed to unsubscribe from channel %s: %v", channel, err)
			}
		} else {
			// Still have subscribers, just update the list
			s.subscribers[key] = newSubscribers
		}
	}

	log.Debugf("unsubscribed messageCh from keys: %v", keys)
	return nil
}

// AddConnection stores connection info in Valkey with TTL
// Key pattern: conn:full:{clientID}:{ip}:{urlEncodedOrigin}
func (s *ValkeyStorage) AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error {
	log := log.WithField("prefix", "ValkeyStorage.AddConnection")

	key := fmt.Sprintf("conn:full:%s:%s:%s", conn.ClientID, conn.IP, url.QueryEscape(conn.Origin))

	data := map[string]interface{}{
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
	exactKey := fmt.Sprintf("conn:full:%s:%s:%s", conn.ClientID, conn.IP, url.QueryEscape(conn.Origin))
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
		// Extract origin from key: conn:full:{clientID}:{ip}:{urlEncodedOrigin}
		parts := strings.Split(key, ":")
		if len(parts) < 5 {
			continue
		}

		encodedOrigin := parts[4]
		cachedOrigin, err := url.QueryUnescape(encodedOrigin)
		if err != nil {
			log.Warnf("failed to decode origin from key %s: %v", key, err)
			continue
		}

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

	log.Info("valkey is healthy")
	return nil
}

// Close closes the storage and all associated resources
func (s *ValkeyStorage) Close() error {
	log := log.WithField("prefix", "ValkeyStorage.Close")

	// Close the sharded pub/sub manager
	if s.shardedPubSub != nil {
		if err := s.shardedPubSub.Close(); err != nil {
			log.Errorf("failed to close sharded pub/sub manager: %v", err)
		}
	}

	// Close the redis client
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			log.Errorf("failed to close redis client: %v", err)
			return err
		}
	}

	log.Info("valkeyStorage closed")
	return nil
}
