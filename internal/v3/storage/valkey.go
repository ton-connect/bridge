package storagev3

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ton-connect/bridge/internal/models"
)

type ValkeyStorage struct {
	client      redis.UniversalClient
	pubSubConn  *redis.PubSub
	subscribers map[string][]chan<- models.SseMessage
	subMutex    sync.RWMutex
}

// NewValkeyStorage creates a Valkey-backed storage client.
// Expects a Redis cluster URL (parsed by redis.ParseURL) and requires
// Redis Cluster + Redis 7+ sharded pub/sub. Returns *ValkeyStorage or error.
func NewValkeyStorage(valkeyURI string) (*ValkeyStorage, error) {
	logger := slog.With("prefix", "NewValkeyStorage")

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

	logger.Info("Successfully connected to Valkey/Redis")

	return &ValkeyStorage{
		client:      clusterClient,
		subscribers: make(map[string][]chan<- models.SseMessage),
	}, nil
}

// detectClusterMode checks if the Redis endpoint is in cluster mode
func detectClusterMode(opts *redis.Options) error {
	client := redis.NewClient(opts)
	defer func() {
		if err := client.Close(); err != nil {
			slog.With("prefix", "detectClusterMode").Warn("failed to close temp redis client", "err", err)
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
	logger := slog.With("prefix", "ValkeyStorage.logDiscoveredNodes")

	err := client.ForEachMaster(ctx, func(ctx context.Context, c *redis.Client) error {
		opts := c.Options()
		logger.Info("Discovered master node", "addr", opts.Addr)
		return nil
	})

	if err != nil {
		logger.Warn("Failed to enumerate cluster nodes", "err", err)
	}
}

// Pub publishes a message to Redis and stores it with TTL
func (s *ValkeyStorage) Pub(ctx context.Context, message models.SseMessage, ttl int64) error {
	logger := slog.With("prefix", "ValkeyStorage.Pub")

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

	logger.Debug("published and stored message", "client_id", message.To, "ttl", ttl)
	return nil
}

// Sub subscribes to Redis channels for the given keys and sends historical messages after lastEventId
func (s *ValkeyStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	logger := slog.With("prefix", "ValkeyStorage.Sub")

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

		s.reapExpired(ctx, clientKey, key, now)

		// Get all remaining messages
		messages, err := s.client.ZRange(ctx, clientKey, 0, -1).Result()
		if err != nil {
			if err != redis.Nil {
				logger.Error("failed to get historical messages", "client_id", key, "err", err)
			}
			continue // No messages for this client or error occurred
		}

		// Parse and send historical messages
		for _, msgData := range messages {
			var msg models.SseMessage
			err := json.Unmarshal([]byte(msgData), &msg)
			if err != nil {
				logger.Error("failed to unmarshal historical message", "err", err)
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
			logger.Error("failed to subscribe to additional channels", "err", err)
		}
	}

	logger.Debug("subscribed to channels for keys", "keys", keys)
	return nil
}

// reapBatchSize bounds one sweep round trip. A client's backlog is not bounded by
// anything, so reading it whole would let a reconnect after a long outage allocate every
// stored payload at once and build a single huge delete command.
const reapBatchSize = 256

// reapScript removes expired backlog members and returns exactly the ones it removed.
//
// The read and the delete have to be one server-side operation. Doing them as two client
// commands lets a concurrent sweep on the other replica remove members between them, and
// then this pod counts messages it did not remove. Inside a script the pair is atomic, so
// the returned members are precisely this call's, which is what makes the count safe to
// add to a metric.
var reapScript = redis.NewScript(`
local members = redis.call('ZRANGEBYSCORE', KEYS[1], '0', ARGV[1], 'LIMIT', 0, tonumber(ARGV[2]))
if #members > 0 then
  redis.call('ZREM', KEYS[1], unpack(members))
end
return members
`)

func deliveredKey(clientID string) string {
	return fmt.Sprintf("delivered:%s", clientID)
}

// MarkDelivered records the delivery in Valkey rather than in process memory.
//
// This is the whole reason the expiry count can be trusted here. The backlog is shared by
// every replica, so a mark kept in one pod's memory is invisible to the pod that later
// sweeps the same client, and every message delivered by one and swept by the other would
// be counted as lost. Keeping the mark beside the data it describes removes that skew by
// construction rather than by hoping one pod does both halves.
func (s *ValkeyStorage) MarkDelivered(ctx context.Context, clientID string, eventID int64) error {
	key := deliveredKey(clientID)
	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, key, eventID)
	pipe.Expire(ctx, key, deliveredMarkTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to mark message %d delivered for %s: %w", eventID, clientID, err)
	}
	return nil
}

// reapExpired drops backlog entries whose expiry score has passed and counts the ones that
// nobody received.
//
// Counting every removed member would be wrong: Pub stores a copy of each message whether
// or not a subscriber was listening, and a delivered copy sits in the backlog until its TTL
// like any other. Only members without a delivery mark are messages that actually went
// undelivered, and those are what the metric is about.
//
// What it still cannot see is structural rather than an oversight. A sorted-set member
// carries no TTL of its own, only the key does, so expiry is resolved lazily here instead
// of by a sweeper like the memory backend runs. A client that never reconnects has its
// whole key dropped by Valkey and its messages expire uncounted. Covering those needs a
// sweeper with leader election across replicas.
func (s *ValkeyStorage) reapExpired(ctx context.Context, clientKey, clientID string, now int64) {
	logger := slog.With("prefix", "ValkeyStorage.reapExpired")
	maxScore := fmt.Sprintf("%d", now)

	for {
		raw, err := reapScript.Run(ctx, s.client, []string{clientKey}, maxScore, reapBatchSize).Result()
		if err != nil {
			if err != redis.Nil {
				logger.Error("failed to sweep expired messages", "client_id", clientID, "err", err)
			}
			return
		}

		batch, ok := raw.([]interface{})
		if !ok || len(batch) == 0 {
			return
		}

		s.countUndelivered(ctx, clientID, batch, logger)

		if len(batch) < reapBatchSize {
			return
		}
	}
}

// countUndelivered adds the members of one swept batch that carry no delivery mark.
func (s *ValkeyStorage) countUndelivered(ctx context.Context, clientID string, batch []interface{}, logger *slog.Logger) {
	messages := make([]models.SseMessage, 0, len(batch))
	ids := make([]interface{}, 0, len(batch))

	for _, item := range batch {
		payload, ok := item.(string)
		if !ok {
			continue
		}
		var msg models.SseMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			logger.Error("failed to unmarshal expired message", "client_id", clientID, "err", err)
			continue
		}
		messages = append(messages, msg)
		ids = append(ids, msg.EventId)
	}
	if len(messages) == 0 {
		return
	}

	// A missing delivered set answers false for every id, so a client whose marks have
	// already expired counts as undelivered. That is the safe direction: the metric may
	// overstate loss after an hour of silence, never hide it.
	marked, err := s.client.SMIsMember(ctx, deliveredKey(clientID), ids...).Result()
	if err != nil {
		logger.Error("failed to read delivery marks", "client_id", clientID, "err", err)
		marked = make([]bool, len(ids))
	}

	undelivered := 0
	for i, msg := range messages {
		if i < len(marked) && marked[i] {
			continue
		}
		undelivered++
		traceID := ""
		var bridgeMsg models.BridgeMessage
		if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
			traceID = bridgeMsg.TraceId
		}
		logger.Debug("message expired undelivered", "client_id", clientID, "event_id", msg.EventId, "trace_id", traceID)
	}
	if undelivered > 0 {
		expiredMessagesMetric.Add(float64(undelivered))
	}
}

// Unsub unsubscribes from Redis channels for the given keys
func (s *ValkeyStorage) Unsub(ctx context.Context, keys []string, messageCh chan<- models.SseMessage) error {
	logger := slog.With("prefix", "ValkeyStorage.Unsub")

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

	logger.Debug("unsubscribed messageCh from keys", "keys", keys, "redis_channels_unsubbed", channelsToUnsub)
	return nil
}

// handlePubSub processes incoming Redis pub-sub messages
func (s *ValkeyStorage) handlePubSub() {
	logger := slog.With("prefix", "ValkeyStorage.handlePubSub")

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
			logger.Error("failed to unmarshal pub-sub message", "err", err)
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
// Key pattern: conn:full:{clientID}:{ip}:{urlEncodedOrigin}
func (s *ValkeyStorage) AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error {
	logger := slog.With("prefix", "ValkeyStorage.AddConnection")

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

	logger.Debug("stored connection", "client_id", conn.ClientID, "ip", conn.IP)
	return nil
}

// VerifyConnection checks if connection matches cached data
// Returns: "ok" (exact match), "warning" (same origin different IP), "danger" (different origin), or "unknown" (no cached data)
func (s *ValkeyStorage) VerifyConnection(ctx context.Context, conn ConnectionInfo) (string, error) {
	logger := slog.With("prefix", "ValkeyStorage.VerifyConnection")

	// Check for exact match first
	exactKey := fmt.Sprintf("conn:full:%s:%s:%s", conn.ClientID, conn.IP, url.QueryEscape(conn.Origin))
	exists, err := s.client.Exists(ctx, exactKey).Result()
	if err != nil {
		return "", fmt.Errorf("failed to check connection existence: %w", err)
	}
	if exists > 0 {
		logger.Debug("connection verified OK", "client_id", conn.ClientID)
		return "ok", nil
	}

	// Get all connections for this clientID
	indexKey := fmt.Sprintf("conn:idx:%s", conn.ClientID)
	keys, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		if err == redis.Nil {
			logger.Debug("no cached connections", "client_id", conn.ClientID)
			return "unknown", nil
		}
		return "", fmt.Errorf("failed to get connection index: %w", err)
	}

	if len(keys) == 0 {
		logger.Debug("no cached connections", "client_id", conn.ClientID)
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
			logger.Warn("failed to decode origin from key", "key", key, "err", err)
			continue
		}

		if cachedOrigin == conn.Origin {
			leastSuspicious = "warning"
		}
	}

	logger.Debug("connection verification result", "result", leastSuspicious, "client_id", conn.ClientID)
	return leastSuspicious, nil
}

// HealthCheck verifies the connection to Valkey
func (s *ValkeyStorage) HealthCheck() error {
	logger := slog.With("prefix", "ValkeyStorage.HealthCheck")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("valkey health check failed: %w", err)
	}

	logger.Info("Valkey is healthy")
	return nil
}
