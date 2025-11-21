package storagev3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/models"
)

var expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
	Name: "number_of_expired_messages",
	Help: "The total number of expired messages",
})

type MemStorage struct {
	db           map[string][]message
	subscribers  map[string][]chan<- models.SseMessage
	connections  map[string][]memConnection // clientID -> connections
	lock         sync.Mutex
	analytics    analytics.EventCollector
	eventBuilder analytics.EventBuilder
}

type message struct {
	models.SseMessage
	expireAt time.Time
}

type memConnection struct {
	IP        string
	Origin    string
	UserAgent string
	ExpiresAt time.Time
}

func (m message) IsExpired(now time.Time) bool {
	return m.expireAt.Before(now)
}

func NewMemStorage(collector analytics.EventCollector, builder analytics.EventBuilder) *MemStorage {
	s := MemStorage{
		db:           map[string][]message{},
		subscribers:  make(map[string][]chan<- models.SseMessage),
		connections:  make(map[string][]memConnection),
		analytics:    collector,
		eventBuilder: builder,
	}
	go s.watcher()
	return &s
}

func removeExpiredMessages(ms []message, now time.Time) ([]message, []message) {
	results := make([]message, 0)
	expired := make([]message, 0)
	for _, m := range ms {
		if m.IsExpired(now) {
			if !ExpiredCache.IsMarked(m.EventId) {
				expired = append(expired, m)
			}
		} else {
			results = append(results, m)
		}
	}
	return results, expired
}

func (s *MemStorage) watcher() {
	for {
		s.lock.Lock()
		for key, msgs := range s.db {
			actual, expired := removeExpiredMessages(msgs, time.Now())
			s.db[key] = actual

			for _, m := range expired {
				var bridgeMsg models.BridgeMessage
				fromID := "unknown"
				hash := sha256.Sum256(m.Message)
				messageHash := hex.EncodeToString(hash[:])

				if err := json.Unmarshal(m.Message, &bridgeMsg); err == nil {
					fromID = bridgeMsg.From
					contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
					messageHash = hex.EncodeToString(contentHash[:])
				}
				expiredMessagesMetric.Inc()
				logrus.WithFields(map[string]interface{}{
					"hash":     messageHash,
					"from":     fromID,
					"to":       key,
					"event_id": m.EventId,
					"trace_id": bridgeMsg.TraceId,
				}).Debug("message expired")

				_ = s.analytics.TryAdd(s.eventBuilder.NewBridgeMessageExpiredEvent(
					key,
					bridgeMsg.TraceId,
					m.EventId,
					messageHash,
				))
			}
		}
		s.lock.Unlock()
		time.Sleep(time.Second)
	}
}

// Pub publishes a message to all subscribers and stores it with TTL
func (s *MemStorage) Pub(ctx context.Context, mes models.SseMessage, ttl int64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Store message with TTL
	s.db[mes.To] = append(s.db[mes.To], message{
		SseMessage: mes,
		expireAt:   time.Now().Add(time.Duration(ttl) * time.Second),
	})

	// Send to all subscribers for this key
	if subscribers, exists := s.subscribers[mes.To]; exists {
		for _, ch := range subscribers {
			select {
			case ch <- mes:
			default:
				// Channel is full or closed, skip
			}
		}
	}

	return nil
}

// Sub subscribes to messages for the given keys and sends historical messages after lastEventId
func (s *MemStorage) Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Add to subscribers
	for _, key := range keys {
		if s.subscribers[key] == nil {
			s.subscribers[key] = make([]chan<- models.SseMessage, 0)
		}
		s.subscribers[key] = append(s.subscribers[key], messageCh)
	}

	// Retrieve messages
	now := time.Now()
	for _, key := range keys {
		messages, exists := s.db[key]
		if !exists {
			continue
		}

		for _, msg := range messages {
			if msg.IsExpired(now) {
				continue
			}
			if msg.EventId <= lastEventId {
				continue
			}

			select {
			case messageCh <- msg.SseMessage:
			default:
				// Channel is full or closed, skip
			}
		}
	}

	return nil
}

// Unsub unsubscribes from messages for the given keys
func (s *MemStorage) Unsub(ctx context.Context, keys []string, messageCh chan<- models.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, key := range keys {
		subscribers, exists := s.subscribers[key]
		if !exists {
			continue
		}

		// Remove only the specific messageCh
		newSubscribers := make([]chan<- models.SseMessage, 0, len(subscribers))
		for _, ch := range subscribers {
			if ch != messageCh {
				newSubscribers = append(newSubscribers, ch)
			}
		}

		if len(newSubscribers) == 0 {
			delete(s.subscribers, key)
		} else {
			s.subscribers[key] = newSubscribers
		}
	}

	return nil
}

// HealthCheck should be implemented
func (s *MemStorage) HealthCheck() error {
	return nil // Always healthy
}

// AddConnection stores connection info in memory with TTL
func (s *MemStorage) AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	memConn := memConnection{
		IP:        conn.IP,
		Origin:    conn.Origin,
		UserAgent: conn.UserAgent,
		ExpiresAt: time.Now().Add(ttl),
	}

	s.connections[conn.ClientID] = append(s.connections[conn.ClientID], memConn)
	return nil
}

// VerifyConnection checks if connection matches cached data
// Returns: "ok" (exact match), "warning" (same origin different IP), "danger" (different origin), or "unknown" (no cached data)
func (s *MemStorage) VerifyConnection(ctx context.Context, conn ConnectionInfo) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	conns, exists := s.connections[conn.ClientID]
	if !exists {
		return "unknown", nil
	}

	now := time.Now()
	foundCount := 0
	leastSuspicious := "danger"

	for _, cachedConn := range conns {
		if now.After(cachedConn.ExpiresAt) {
			continue
		}

		foundCount++

		if cachedConn.Origin == conn.Origin && cachedConn.IP == conn.IP {
			return "ok", nil
		}

		if cachedConn.Origin == conn.Origin {
			leastSuspicious = "warning"
		}
	}

	if foundCount == 0 {
		return "unknown", nil
	}

	return leastSuspicious, nil
}
