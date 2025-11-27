package storagev3

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal/models"
	"github.com/valkey-io/valkey-go"
)

// ShardedPubSubManager handles sharded pub/sub using valkey-go library
// This solves the go-redis limitation where SSUBSCRIBE only works with channels on the same shard
type ShardedPubSubManager struct {
	client       valkey.Client
	subscribers  map[string][]chan<- models.SseMessage
	subMutex     sync.RWMutex
	cancelFuncs  map[string]context.CancelFunc // Per-channel cancel functions
	cancelMutex  sync.Mutex
	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewShardedPubSubManager creates a new manager using valkey-go for sharded pub/sub
func NewShardedPubSubManager(clusterAddrs []string, username, password string) (*ShardedPubSubManager, error) {
	log := log.WithField("prefix", "NewShardedPubSubManager")

	// Create valkey-go cluster client
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: clusterAddrs,
		Username:    username,
		Password:    password,
		// Enable auto pipelining for better performance
		DisableAutoPipelining: false,
		// Shuffle initial addresses for better load distribution
		ShuffleInit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create valkey client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping valkey cluster: %w", err)
	}

	log.Info("successfully created valkey-go sharded pub/sub manager")

	return &ShardedPubSubManager{
		client:       client,
		subscribers:  make(map[string][]chan<- models.SseMessage),
		cancelFuncs:  make(map[string]context.CancelFunc),
		shutdownChan: make(chan struct{}),
	}, nil
}

// Subscribe subscribes to a channel using SSUBSCRIBE
// valkey-go handles the complexity of routing to the correct shard
func (m *ShardedPubSubManager) Subscribe(ctx context.Context, channel string, messageCh chan<- models.SseMessage) error {
	log := log.WithField("prefix", "ShardedPubSubManager.Subscribe")

	m.subMutex.Lock()
	defer m.subMutex.Unlock()

	// Add subscriber to the list
	if m.subscribers[channel] == nil {
		m.subscribers[channel] = make([]chan<- models.SseMessage, 0)
	}
	m.subscribers[channel] = append(m.subscribers[channel], messageCh)

	// If this is the first subscriber for this channel, start a dedicated receiver
	if len(m.subscribers[channel]) == 1 {
		if err := m.startChannelReceiver(channel); err != nil {
			// Remove the subscriber we just added since we failed to start the receiver
			m.subscribers[channel] = m.subscribers[channel][:len(m.subscribers[channel])-1]
			if len(m.subscribers[channel]) == 0 {
				delete(m.subscribers, channel)
			}
			return fmt.Errorf("failed to start receiver for channel %s: %w", channel, err)
		}
	}

	log.Debugf("subscribed to channel %s (total subscribers: %d)", channel, len(m.subscribers[channel]))
	return nil
}

// startChannelReceiver starts a dedicated goroutine to receive messages for a specific channel
func (m *ShardedPubSubManager) startChannelReceiver(channel string) error {
	log := log.WithField("prefix", "ShardedPubSubManager.startChannelReceiver")

	// Create a cancellable context for this channel's receiver
	ctx, cancel := context.WithCancel(context.Background())

	m.cancelMutex.Lock()
	m.cancelFuncs[channel] = cancel
	m.cancelMutex.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.cancelMutex.Lock()
			delete(m.cancelFuncs, channel)
			m.cancelMutex.Unlock()
		}()

		log.Infof("starting receiver for channel: %s", channel)

		// Use valkey-go's Receive method with SSUBSCRIBE
		// This will automatically handle the correct shard routing
		err := m.client.Receive(ctx, m.client.B().Ssubscribe().Channel(channel).Build(), func(msg valkey.PubSubMessage) {
			// Parse the message
			var sseMessage models.SseMessage
			if err := json.Unmarshal([]byte(msg.Message), &sseMessage); err != nil {
				log.Errorf("failed to unmarshal message from channel %s: %v", channel, err)
				return
			}

			// Distribute to all subscribers
			m.subMutex.RLock()
			subscribers := m.subscribers[channel]
			for _, ch := range subscribers {
				select {
				case ch <- sseMessage:
				default:
					// Channel is full or closed, skip
					log.Warnf("failed to send message to subscriber on channel %s (channel full or closed)", channel)
				}
			}
			m.subMutex.RUnlock()
		})

		if err != nil && err != context.Canceled {
			log.Errorf("receiver for channel %s stopped with error: %v", channel, err)
		} else {
			log.Infof("receiver for channel %s stopped", channel)
		}
	}()

	return nil
}

// Unsubscribe removes a subscriber from a channel
func (m *ShardedPubSubManager) Unsubscribe(channel string, messageCh chan<- models.SseMessage) error {
	log := log.WithField("prefix", "ShardedPubSubManager.Unsubscribe")

	m.subMutex.Lock()
	defer m.subMutex.Unlock()

	subscribers, exists := m.subscribers[channel]
	if !exists {
		return nil
	}

	// Remove the specific messageCh from subscribers
	newSubscribers := make([]chan<- models.SseMessage, 0, len(subscribers))
	for _, ch := range subscribers {
		if ch != messageCh {
			newSubscribers = append(newSubscribers, ch)
		}
	}

	if len(newSubscribers) == 0 {
		// No more subscribers for this channel, stop the receiver
		delete(m.subscribers, channel)

		m.cancelMutex.Lock()
		if cancel, exists := m.cancelFuncs[channel]; exists {
			cancel()
		}
		m.cancelMutex.Unlock()

		log.Debugf("unsubscribed last subscriber from channel %s, stopped receiver", channel)
	} else {
		// Still have subscribers, just update the list
		m.subscribers[channel] = newSubscribers
		log.Debugf("unsubscribed from channel %s (remaining subscribers: %d)", channel, len(newSubscribers))
	}

	return nil
}

// Publish publishes a message to a channel using SPUBLISH
func (m *ShardedPubSubManager) Publish(ctx context.Context, channel string, message []byte) error {
	return m.client.Do(ctx, m.client.B().Spublish().Channel(channel).Message(string(message)).Build()).Error()
}

// Close closes the manager and stops all receivers
func (m *ShardedPubSubManager) Close() error {
	log := log.WithField("prefix", "ShardedPubSubManager.Close")

	close(m.shutdownChan)

	// Cancel all receivers
	m.cancelMutex.Lock()
	for _, cancel := range m.cancelFuncs {
		cancel()
	}
	m.cancelMutex.Unlock()

	// Wait for all receivers to finish
	m.wg.Wait()

	// Close the valkey client
	m.client.Close()

	log.Info("shardedPubSubManager closed")
	return nil
}
