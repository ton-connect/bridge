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
	"github.com/tonkeeper/bridge/internal/config"
	"github.com/tonkeeper/bridge/internal/models"
)

type ValkeyStorage struct {
	client      redis.UniversalClient
	cluster     *redis.ClusterClient
	isCluster   bool
	pubSubConn  *redis.PubSub
	subscribers map[string][]chan<- models.SseMessage
	subMutex    sync.RWMutex
}

// NewValkeyStorage creates a new Valkey storage instance
// Supports both single node and cluster modes based on URI format
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	log := log.WithField("prefix", "NewValkeyStorage")

	uris := strings.Split(valkeyURI, ",")

	var client redis.UniversalClient
	var clusterClient *redis.ClusterClient
	isCluster := false

	// Determine cluster mode:
	// - If multiple URIs provided -> cluster
	// - If single URI but looks like AWS ElastiCache cluster endpoint (contains "clustercfg") -> cluster
	if len(uris) > 1 {
		isCluster = true
	} else if strings.Contains(strings.ToLower(valkeyURI), "clustercfg") {
		isCluster = true
	}

	if isCluster {
		var addrs []string
		var firstOpts *redis.Options
		if len(uris) > 1 {
			addrs = make([]string, len(uris))
			for i, uri := range uris {
				opts, err := redis.ParseURL(strings.TrimSpace(uri))
				if err != nil {
					return nil, fmt.Errorf("failed to parse URI %d: %w", i+1, err)
				}
				addrs[i] = opts.Addr
				if i == 0 {
					firstOpts = opts
				}
			}
		} else {
			raw := strings.TrimSpace(uris[0])
			opts, err := redis.ParseURL(raw)
			if err != nil {
				return nil, fmt.Errorf("failed to parse URI: %w", err)
			}
			// For AWS clustercfg endpoints, seed with hostname only; go-redis will discover node addresses
			u, _ := url.Parse(raw)
			seed := opts.Addr
			if u != nil && strings.Contains(strings.ToLower(u.Host), "clustercfg") {
				seed = u.Host
			}
			addrs = []string{seed}
			firstOpts = opts
		}

		log.Infof("Using cluster mode with %d node seed(s)", len(addrs))

		// Parse timeout durations from config
		readTimeout, err := time.ParseDuration(config.Config.ValkeyReadTimeout)
		if err != nil {
			log.Warnf("invalid VALKEY_READ_TIMEOUT '%s', using default 30s: %v", config.Config.ValkeyReadTimeout, err)
			readTimeout = 30 * time.Second
		}
		writeTimeout, err := time.ParseDuration(config.Config.ValkeyWriteTimeout)
		if err != nil {
			log.Warnf("invalid VALKEY_WRITE_TIMEOUT '%s', using default 30s: %v", config.Config.ValkeyWriteTimeout, err)
			writeTimeout = 30 * time.Second
		}
		dialTimeout, err := time.ParseDuration(config.Config.ValkeyDialTimeout)
		if err != nil {
			log.Warnf("invalid VALKEY_DIAL_TIMEOUT '%s', using default 10s: %v", config.Config.ValkeyDialTimeout, err)
			dialTimeout = 10 * time.Second
		}
		poolTimeout, err := time.ParseDuration(config.Config.ValkeyPoolTimeout)
		if err != nil {
			log.Warnf("invalid VALKEY_POOL_TIMEOUT '%s', using default 30s: %v", config.Config.ValkeyPoolTimeout, err)
			poolTimeout = 30 * time.Second
		}

		clusterClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:          addrs,
			Password:       firstOpts.Password,
			Username:       firstOpts.Username,
			TLSConfig:      firstOpts.TLSConfig,
			ReadOnly:       config.Config.ValkeyReadOnly,
			RouteByLatency: config.Config.ValkeyRouteByLatency,
			RouteRandomly:  config.Config.ValkeyRouteRandomly,
			MaxRedirects:   config.Config.ValkeyMaxRedirects,
			ReadTimeout:    readTimeout,
			WriteTimeout:   writeTimeout,
			DialTimeout:    dialTimeout,
			PoolTimeout:    poolTimeout,
		})
		client = clusterClient
	} else {
		opts, err := redis.ParseURL(strings.TrimSpace(uris[0]))
		if err != nil {
			return nil, fmt.Errorf("failed to parse URI: %w", err)
		}
		log.Info("Using single-node mode")
		client = redis.NewClient(opts)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	log.Info("Successfully connected to Valkey")
	return &ValkeyStorage{
		client:      client,
		cluster:     clusterClient,
		isCluster:   isCluster,
		subscribers: make(map[string][]chan<- models.SseMessage),
	}, nil
}

// Pub publishes a message to Redis and stores it with TTL
func (s *ValkeyStorage) Pub(ctx context.Context, message models.SseMessage, ttl int64) error {
	log := log.WithField("prefix", "ValkeyStorage.Pub")

	// Publish to Redis channel (use hash-tag for slot affinity)
	channel := clientKey(message.To)
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if s.isCluster && s.cluster != nil {
		err = s.cluster.SPublish(ctx, channel, messageData).Err()
	} else {
		err = s.client.Publish(ctx, channel, messageData).Err()
	}
	if err != nil {
		return fmt.Errorf("failed to publish message to channel %s: %w", channel, err)
	}

	// Store message with TTL as backup for offline clients
	expireTime := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	primaryKey := clientKey(message.To)
	err = s.client.ZAdd(ctx, primaryKey, redis.Z{
		Score:  float64(expireTime),
		Member: messageData,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to store message in sorted set for channel %s: %w", channel, err)
	}

	// Set expiration on the key itself
	s.client.Expire(ctx, primaryKey, time.Duration(ttl+60)*time.Second) // TODO remove 60 seconds buffer?

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
		clientKey := clientKey(key)

		// Remove expired messages first
		// TODO support expired messages but not delivered log
		s.client.ZRemRangeByScore(ctx, clientKey, "0", fmt.Sprintf("%d", now))

		// Get all remaining messages
		messages, err := s.client.ZRange(ctx, clientKey, 0, -1).Result()
		if err != nil {
			if err != redis.Nil {
				log.Errorf("failed to get historical messages for %s: %v", clientKey, err)
			}
			messages = []string{}
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
		channels[i] = clientKey(key)
	}

	// If this is the first subscription, start the pub-sub connection
	if s.pubSubConn == nil {
		if s.isCluster && s.cluster != nil {
			s.pubSubConn = s.cluster.SSubscribe(ctx, channels...)
		} else {
			s.pubSubConn = s.client.Subscribe(ctx, channels...)
		}
		go s.handlePubSub()
	} else {
		// Subscribe to additional channels
		var err error
		if s.isCluster && s.cluster != nil {
			err = s.pubSubConn.SSubscribe(ctx, channels...)
		} else {
			err = s.pubSubConn.Subscribe(ctx, channels...)
		}
		if err != nil {
			log.Errorf("failed to subscribe to additional channels: %v", err)
		}
	}

	log.Debugf("subscribed to channels for keys: %v", keys)
	return nil
}

// Unsub unsubscribes from Redis channels for the given keys
func (s *ValkeyStorage) Unsub(ctx context.Context, keys []string) error {
	log := log.WithField("prefix", "ValkeyStorage.Unsub")

	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	channels := make([]string, 0)
	for _, key := range keys {
		channel := clientKey(key)
		channels = append(channels, channel)
		delete(s.subscribers, key)
	}

	if s.pubSubConn != nil {
		var err error
		if s.isCluster && s.cluster != nil {
			err = s.pubSubConn.SUnsubscribe(ctx, channels...)
		} else {
			err = s.pubSubConn.Unsubscribe(ctx, channels...)
		}
		if err != nil {
			return fmt.Errorf("failed to unsubscribe from channels: %w", err)
		}
	}

	log.Debugf("unsubscribed from channels for keys: %v", keys)
	return nil
}

// handlePubSub processes incoming Redis pub-sub messages
func (s *ValkeyStorage) handlePubSub() {
	log := log.WithField("prefix", "ValkeyStorage.handlePubSub")

	for msg := range s.pubSubConn.Channel() {
		// Parse channel name to get client key
		// Extract original client id from channel name, which is formatted as client:{<id>}
		// We need to remove the prefix and hash-tag braces
		var key string
		if len(msg.Channel) > 7 && strings.HasPrefix(msg.Channel, "client:") {
			ch := msg.Channel[7:]
			// Remove surrounding braces if present
			if len(ch) >= 2 && strings.HasPrefix(ch, "{") && strings.HasSuffix(ch, "}") {
				key = ch[1 : len(ch)-1]
			} else {
				key = ch
			}
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

func clientKey(id string) string {
	return fmt.Sprintf("client:{%s}", id)
}
