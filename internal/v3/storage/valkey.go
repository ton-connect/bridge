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

// CRC16 lookup table for Redis cluster key hashing
var crc16tab = [256]uint16{
	0x0000, 0x1021, 0x2042, 0x3063, 0x4084, 0x50a5, 0x60c6, 0x70e7,
	0x8108, 0x9129, 0xa14a, 0xb16b, 0xc18c, 0xd1ad, 0xe1ce, 0xf1ef,
	0x1231, 0x0210, 0x3273, 0x2252, 0x52b5, 0x4294, 0x72f7, 0x62d6,
	0x9339, 0x8318, 0xb37b, 0xa35a, 0xd3bd, 0xc39c, 0xf3ff, 0xe3de,
	0x2462, 0x3443, 0x0420, 0x1401, 0x64e6, 0x74c7, 0x44a4, 0x5485,
	0xa56a, 0xb54b, 0x8528, 0x9509, 0xe5ee, 0xf5cf, 0xc5ac, 0xd58d,
	0x3653, 0x2672, 0x1611, 0x0630, 0x76d7, 0x66f6, 0x5695, 0x46b4,
	0xb75b, 0xa77a, 0x9719, 0x8738, 0xf7df, 0xe7fe, 0xd79d, 0xc7bc,
	0x48c4, 0x58e5, 0x6886, 0x78a7, 0x0840, 0x1861, 0x2802, 0x3823,
	0xc9cc, 0xd9ed, 0xe98e, 0xf9af, 0x8948, 0x9969, 0xa90a, 0xb92b,
	0x5af5, 0x4ad4, 0x7ab7, 0x6a96, 0x1a71, 0x0a50, 0x3a33, 0x2a12,
	0xdbfd, 0xcbdc, 0xfbbf, 0xeb9e, 0x9b79, 0x8b58, 0xbb3b, 0xab1a,
	0x6ca6, 0x7c87, 0x4ce4, 0x5cc5, 0x2c22, 0x3c03, 0x0c60, 0x1c41,
	0xedae, 0xfd8f, 0xcdec, 0xddcd, 0xad2a, 0xbd0b, 0x8d68, 0x9d49,
	0x7e97, 0x6eb6, 0x5ed5, 0x4ef4, 0x3e13, 0x2e32, 0x1e51, 0x0e70,
	0xff9f, 0xefbe, 0xdfdd, 0xcffc, 0xbf1b, 0xaf3a, 0x9f59, 0x8f78,
	0x9188, 0x81a9, 0xb1ca, 0xa1eb, 0xd10c, 0xc12d, 0xf14e, 0xe16f,
	0x1080, 0x00a1, 0x30c2, 0x20e3, 0x5004, 0x4025, 0x7046, 0x6067,
	0x83b9, 0x9398, 0xa3fb, 0xb3da, 0xc33d, 0xd31c, 0xe37f, 0xf35e,
	0x02b1, 0x1290, 0x22f3, 0x32d2, 0x4235, 0x5214, 0x6277, 0x7256,
	0xb5ea, 0xa5cb, 0x95a8, 0x8589, 0xf56e, 0xe54f, 0xd52c, 0xc50d,
	0x34e2, 0x24c3, 0x14a0, 0x0481, 0x7466, 0x6447, 0x5424, 0x4405,
	0xa7db, 0xb7fa, 0x8799, 0x97b8, 0xe75f, 0xf77e, 0xc71d, 0xd73c,
	0x26d3, 0x36f2, 0x0691, 0x16b0, 0x6657, 0x7676, 0x4615, 0x5634,
	0xd94c, 0xc96d, 0xf90e, 0xe92f, 0x99c8, 0x89e9, 0xb98a, 0xa9ab,
	0x5844, 0x4865, 0x7806, 0x6827, 0x18c0, 0x08e1, 0x3882, 0x28a3,
	0xcb7d, 0xdb5c, 0xeb3f, 0xfb1e, 0x8bf9, 0x9bd8, 0xabbb, 0xbb9a,
	0x4a75, 0x5a54, 0x6a37, 0x7a16, 0x0af1, 0x1ad0, 0x2ab3, 0x3a92,
	0xfd2e, 0xed0f, 0xdd6c, 0xcd4d, 0xbdaa, 0xad8b, 0x9de8, 0x8dc9,
	0x7c26, 0x6c07, 0x5c64, 0x4c45, 0x3ca2, 0x2c83, 0x1ce0, 0x0cc1,
	0xef1f, 0xff3e, 0xcf5d, 0xdf7c, 0xaf9b, 0xbfba, 0x8fd9, 0x9ff8,
	0x6e17, 0x7e36, 0x4e55, 0x5e74, 0x2e93, 0x3eb2, 0x0ed1, 0x1ef0,
}

// redisClusterSlot calculates the hash slot for a Redis key using CRC16
// This matches Redis CLUSTER KEYSLOT algorithm: CRC16(key) mod 16384
func redisClusterSlot(key string) uint16 {
	// Extract the hashtag if present: {tag}
	// Redis only hashes the part between the first { and first } after it
	start := strings.Index(key, "{")
	if start != -1 {
		end := strings.Index(key[start+1:], "}")
		if end != -1 {
			key = key[start+1 : start+1+end]
		}
	}

	// Calculate CRC16 XMODEM
	var crc uint16 = 0
	for i := 0; i < len(key); i++ {
		crc = (crc << 8) ^ crc16tab[((crc>>8)^uint16(key[i]))&0x00FF]
	}

	return crc % 16384
}

type ValkeyStorage struct {
	client       redis.UniversalClient
	pubSubConn   *redis.PubSub         // used in single-node mode
	pubSubConns  map[int]*redis.PubSub // used in cluster mode: one per shard
	subscribers  map[string][]chan<- models.SseMessage
	subMutex     sync.RWMutex
	isCluster    bool                // true if running in cluster mode
	clusterSlots []redis.ClusterSlot // cache cluster topology
}

// getShardForChannel determines which shard index owns a given channel
// Returns the index in clusterSlots array, or 0 for single-node mode
func (s *ValkeyStorage) getShardForChannel(channel string) int {
	if !s.isCluster || len(s.clusterSlots) == 0 {
		return 0 // single-node mode
	}

	// Calculate slot for this channel
	slot := redisClusterSlot(channel)

	// Find which shard owns this slot
	for i, clusterSlot := range s.clusterSlots {
		if int(slot) >= clusterSlot.Start && int(slot) <= clusterSlot.End {
			return i
		}
	}

	// Fallback to shard 0 if not found (shouldn't happen in healthy cluster)
	log.Warnf("slot %d for channel %s not found in cluster slots, using shard 0", slot, channel)
	return 0
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
		isClusterMode := false

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}

		log.Info("Successfully connected to Valkey (single-node)")
		return &ValkeyStorage{
			client:       client,
			subscribers:  make(map[string][]chan<- models.SseMessage),
			isCluster:    isClusterMode,
			pubSubConns:  make(map[int]*redis.PubSub),
			clusterSlots: []redis.ClusterSlot{},
		}, nil
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
		isClusterMode := true

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("connection failed: %w", err)
		}

		log.Info("Successfully connected to Valkey (cluster mode, sharded pub/sub enabled)")
		return &ValkeyStorage{
			client:       client,
			subscribers:  make(map[string][]chan<- models.SseMessage),
			isCluster:    isClusterMode,
			pubSubConns:  make(map[int]*redis.PubSub),
			clusterSlots: clusterSlots, // cache cluster topology for shard routing
		}, nil
	}
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

	log.Debugf("attempting to spublish message to channel %s", channel)
	err = s.client.SPublish(ctx, channel, messageData).Err()
	if err != nil {
		log.Errorf("SPUBLISH failed for channel %s: %v", channel, err)
		return fmt.Errorf("failed to publish message to shard channel %s: %w", channel, err)
	}
	log.Debugf("SPUBLISH successful for channel %s", channel)

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

	log.Debugf("published and stored message for client %s with TTL %d seconds (sharded pub/sub)", message.To, ttl)
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

	// Handle subscription based on cluster mode
	if s.isCluster && len(s.clusterSlots) > 0 {
		// Cluster mode: group channels by shard
		channelsByShard := make(map[int][]string)
		for _, channel := range channels {
			shard := s.getShardForChannel(channel)
			channelsByShard[shard] = append(channelsByShard[shard], channel)
		}

		// Subscribe on each shard's connection
		for shard, shardChannels := range channelsByShard {
			if s.pubSubConns[shard] == nil {
				log.Debugf("creating new PubSub connection for shard %d with channels: %v", shard, shardChannels)
				s.pubSubConns[shard] = s.client.SSubscribe(ctx, shardChannels...)
				go s.handlePubSubForShard(shard)
			} else {
				log.Debugf("adding channels to existing shard %d connection: %v", shard, shardChannels)
				err := s.pubSubConns[shard].SSubscribe(ctx, shardChannels...)
				if err != nil {
					log.Errorf("failed to ssubscribe to additional channels on shard %d: %v", shard, err)
				}
			}
		}
		log.Debugf("ssubscribed to shard channels for keys: %v (distributed across %d shards)", keys, len(channelsByShard))
	} else {
		// Single-node mode: use single connection
		if s.pubSubConn == nil {
			log.Debugf("creating new PubSub connection (single-node) with channels: %v", channels)
			s.pubSubConn = s.client.SSubscribe(ctx, channels...)
			go s.handlePubSub()
		} else {
			// Subscribe to additional channels
			err := s.pubSubConn.SSubscribe(ctx, channels...)
			if err != nil {
				log.Errorf("failed to ssubscribe to additional channels: %v", err)
			}
		}
		log.Debugf("ssubscribed to shard channels for keys: %v (single-node mode)", keys)
	}

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
	if len(channelsToUnsub) > 0 {
		if s.isCluster && len(s.clusterSlots) > 0 {
			// Cluster mode: group channels by shard and unsubscribe from each
			channelsByShard := make(map[int][]string)
			for _, channel := range channelsToUnsub {
				shard := s.getShardForChannel(channel)
				channelsByShard[shard] = append(channelsByShard[shard], channel)
			}

			// Unsubscribe from each shard
			for shard, shardChannels := range channelsByShard {
				if s.pubSubConns[shard] != nil {
					log.Debugf("unsubscribing from shard %d channels: %v", shard, shardChannels)
					err := s.pubSubConns[shard].SUnsubscribe(ctx, shardChannels...)
					if err != nil {
						log.Errorf("failed to sunsubscribe from shard %d channels: %v", shard, err)
						return fmt.Errorf("failed to sunsubscribe from shard %d channels: %w", shard, err)
					}
				}
			}
			log.Debugf("sunsubscribed messageCh from keys: %v (redis shard channels unsubbed across %d shards: %v)", keys, len(channelsByShard), channelsToUnsub)
		} else {
			// Single-node mode: use single connection
			if s.pubSubConn != nil {
				err := s.pubSubConn.SUnsubscribe(ctx, channelsToUnsub...)
				if err != nil {
					return fmt.Errorf("failed to sunsubscribe from channels: %w", err)
				}
			}
			log.Debugf("sunsubscribed messageCh from keys: %v (redis shard channels unsubbed: %v)", keys, channelsToUnsub)
		}
	}

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

// handlePubSubForShard processes incoming Redis pub-sub messages from a specific shard
func (s *ValkeyStorage) handlePubSubForShard(shardID int) {
	log := log.WithFields(map[string]interface{}{
		"prefix": "ValkeyStorage.handlePubSubForShard",
		"shard":  shardID,
	})

	log.Infof("started PubSub handler for shard %d", shardID)

	for msg := range s.pubSubConns[shardID].Channel() {
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
			log.Errorf("failed to unmarshal pub-sub message from shard %d: %v", shardID, err)
			continue
		}

		log.Debugf("received message from shard %d for key %s, eventId %d", shardID, key, sseMessage.EventId)

		// Send to all subscribers for this key
		s.subMutex.RLock()
		subscribers, exists := s.subscribers[key]
		if exists {
			for _, ch := range subscribers {
				select {
				case ch <- sseMessage:
				default:
					// Channel is full or closed, skip
					log.Warnf("failed to send message to subscriber for key %s (channel full or closed)", key)
				}
			}
		} else {
			log.Debugf("no subscribers found for key %s", key)
		}
		s.subMutex.RUnlock()
	}

	log.Warnf("PubSub handler for shard %d stopped (channel closed)", shardID)
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
