package storagev3

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ton-connect/bridge/internal/models"
)

// Sub subscribes a barrier channel of its own after the real ones: a connection's replies come in order,
// so its ack means every earlier SUBSCRIBE and UNSUBSCRIBE there took effect. Past the timeout Sub reads anyway.
const (
	subscribeBarrierPrefix  = "subscribe-barrier:"
	subscribeBarrierTimeout = 5 * time.Second
)

type ValkeyStorage struct {
	client      redis.UniversalClient
	pubSubConn  *redis.PubSub
	subscribers map[string][]*valkeySubscriber
	subMutex    sync.RWMutex

	barrierNonce  string
	barrierSeq    atomic.Uint64
	barriers      map[string]chan struct{}
	barriersMutex sync.Mutex
}

// valkeySubscriber delivers to one session. Live messages are held back until the history replay
// ends, so history comes first as from the memory storage, and an event seen both ways is sent once.
type valkeySubscriber struct {
	ch        chan<- models.SseMessage
	mu        sync.Mutex
	replaying bool
	held      []models.SseMessage
	replayed  map[deliveryKey]struct{}
}

// deliveryKey carries the recipient, since one session can subscribe several client IDs, and a payload
// digest, since event IDs from different instances can collide and the set lives as long as the session.
type deliveryKey struct {
	eventID int64
	to      string
	payload [sha256.Size]byte
}

func keyOf(msg models.SseMessage) deliveryKey {
	return deliveryKey{msg.EventId, msg.To, sha256.Sum256(msg.Message)}
}

func newValkeySubscriber(ch chan<- models.SseMessage) *valkeySubscriber {
	return &valkeySubscriber{ch: ch, replaying: true, replayed: make(map[deliveryKey]struct{})}
}

func (v *valkeySubscriber) deliverLive(msg models.SseMessage) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.replayed[keyOf(msg)]; ok {
		return
	}
	if !v.replaying {
		v.send(msg, DropPathLive)
	} else if len(v.held) < cap(v.ch) {
		v.held = append(v.held, msg)
	} else {
		MessagesDropped.WithLabelValues(DropPathLive, DropReasonSubscriberFull).Inc()
	}
}

func (v *valkeySubscriber) deliverReplay(msg models.SseMessage) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := keyOf(msg)
	if _, ok := v.replayed[key]; ok {
		return
	}
	if v.send(msg, DropPathReplay) {
		v.replayed[key] = struct{}{}
	}
}

// replayDone sends the held live messages but keeps the replayed set: the live copy of a replayed
// event may still be on its way.
func (v *valkeySubscriber) replayDone() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.replaying = false
	for _, msg := range v.held {
		if _, ok := v.replayed[keyOf(msg)]; !ok {
			v.send(msg, DropPathLive)
		}
	}
	v.held = nil
}

func (v *valkeySubscriber) send(msg models.SseMessage, dropPath string) bool {
	select {
	case v.ch <- msg:
		return true
	default:
		MessagesDropped.WithLabelValues(dropPath, DropReasonSubscriberFull).Inc()
		return false
	}
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

	clusterClient := redis.NewClusterClient(clusterOptions(opts))

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

	return newValkeyStorage(clusterClient), nil
}

func clusterOptions(opts *redis.Options) *redis.ClusterOptions {
	return &redis.ClusterOptions{
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
	}
}

func newValkeyStorage(client redis.UniversalClient) *ValkeyStorage {
	return &ValkeyStorage{
		client:       client,
		subscribers:  make(map[string][]*valkeySubscriber),
		barrierNonce: rand.Text(),
		barriers:     make(map[string]chan struct{}),
	}
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

	channel := fmt.Sprintf("client:%s", message.To)
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Stored before it is published: a subscriber that misses the live copy reads history only
	// after its subscription is live, and has to find the message there.
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

	// Publish to Redis channel
	err = s.client.Publish(ctx, channel, messageData).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message to channel %s: %w", channel, err)
	}

	logger.Debug("published and stored message", "client_id", message.To, "ttl", ttl)
	return nil
}

// Sub subscribes to Redis channels for the given keys and sends historical messages after lastEventId.
// History is read only once the subscription is live: a message published before that point is
// then in history, and one published after it arrives live.
func (s *ValkeyStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	logger := slog.With("prefix", "ValkeyStorage.Sub")

	sub := newValkeySubscriber(messageCh)

	// Create channels list for subscription
	channels := make([]string, len(keys))
	for i, key := range keys {
		channels[i] = fmt.Sprintf("client:%s", key)
	}

	s.subMutex.Lock()

	// Add messageCh to subscribers for each key
	for _, key := range keys {
		if s.subscribers[key] == nil {
			s.subscribers[key] = make([]*valkeySubscriber, 0)
		}
		s.subscribers[key] = append(s.subscribers[key], sub)
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

	pubSubConn := s.pubSubConn
	s.subMutex.Unlock()

	if !s.awaitSubscribed(ctx, pubSubConn) {
		logger.Warn("subscription not seen live in time, reading history anyway", "keys", keys)
	}

	// Send historical messages for each key
	now := time.Now().Unix()
	for _, key := range keys {
		clientKey := fmt.Sprintf("client:%s", key)

		// A transaction runs on the slot's primary, and a replica may not have the write of a
		// message that missed the subscription yet.
		var history *redis.StringSliceCmd
		_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			// Remove expired messages first
			// TODO support expired messages but not delivered log
			pipe.ZRemRangeByScore(ctx, clientKey, "0", fmt.Sprintf("%d", now))

			// Get all remaining messages
			history = pipe.ZRange(ctx, clientKey, 0, -1)
			return nil
		})
		if err != nil {
			if err != redis.Nil {
				logger.Error("failed to get historical messages", "client_id", key, "err", err)
			}
			continue // No messages for this client or error occurred
		}
		messages := history.Val()

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
				sub.deliverReplay(msg)
			}
		}
	}
	sub.replayDone()

	logger.Debug("subscribed to channels for keys", "keys", keys)
	return nil
}

// awaitSubscribed waits for the ack of a barrier channel nobody else subscribes. An ack for a real channel
// could answer a previous session's SUBSCRIBE that its UNSUBSCRIBE has since undone.
func (s *ValkeyStorage) awaitSubscribed(ctx context.Context, pubSubConn *redis.PubSub) bool {
	ctx, cancel := context.WithTimeout(ctx, subscribeBarrierTimeout)
	defer cancel()

	barrier := fmt.Sprintf("%s%s:%d", subscribeBarrierPrefix, s.barrierNonce, s.barrierSeq.Add(1))
	acked := make(chan struct{})
	s.barriersMutex.Lock()
	s.barriers[barrier] = acked
	s.barriersMutex.Unlock()
	defer func() {
		s.barriersMutex.Lock()
		delete(s.barriers, barrier)
		s.barriersMutex.Unlock()
		if err := pubSubConn.Unsubscribe(context.Background(), barrier); err != nil {
			slog.Warn("failed to unsubscribe from subscribe barrier", "err", err)
		}
	}()

	if err := pubSubConn.Subscribe(ctx, barrier); err != nil {
		return false
	}
	select {
	case <-acked:
		return true
	case <-ctx.Done():
		return false
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
		newSubscribers := make([]*valkeySubscriber, 0, len(subscribers))
		for _, sub := range subscribers {
			if sub.ch != messageCh {
				newSubscribers = append(newSubscribers, sub)
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

	for m := range s.pubSubConn.ChannelWithSubscriptions() {
		msg, ok := m.(*redis.Message)
		if !ok {
			if sub, ok := m.(*redis.Subscription); ok && sub.Kind == "subscribe" {
				s.barrierAcked(sub.Channel)
			}
			continue
		}

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
			for _, sub := range subscribers {
				sub.deliverLive(sseMessage)
			}
		}
		s.subMutex.RUnlock()
	}
}

// barrierAcked wakes the Sub waiting for this barrier. Acks for real channels, and a barrier acked
// again after go-redis resubscribes, match nothing.
func (s *ValkeyStorage) barrierAcked(channel string) {
	s.barriersMutex.Lock()
	acked, ok := s.barriers[channel]
	delete(s.barriers, channel)
	s.barriersMutex.Unlock()
	if ok {
		close(acked)
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
