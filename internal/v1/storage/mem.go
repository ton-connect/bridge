package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal/models"
)

type MemStorage struct {
	db   map[string][]message
	lock sync.Mutex
}

type message struct {
	models.SseMessage
	expireAt time.Time
}

func (m message) IsExpired(now time.Time) bool {
	return m.expireAt.Before(now)
}

func NewMemStorage() *MemStorage {
	s := MemStorage{
		db: map[string][]message{},
	}
	go s.watcher()
	return &s
}

func removeExpiredMessages(ms []message, now time.Time, clientID string) []message {
	log := log.WithField("prefix", "removeExpiredMessages")
	results := make([]message, 0)
	for _, m := range ms {
		if m.IsExpired(now) {
			if !ExpiredCache.IsMarked(m.EventId) {
				var bridgeMsg models.BridgeMessage
				fromID := "unknown"
				hash := sha256.Sum256(m.Message)
				messageHash := hex.EncodeToString(hash[:])

				if err := json.Unmarshal(m.Message, &bridgeMsg); err == nil {
					fromID = bridgeMsg.From
					contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
					messageHash = hex.EncodeToString(contentHash[:])
				}

				log.WithFields(map[string]interface{}{
					"hash":     messageHash,
					"from":     fromID,
					"to":       clientID,
					"event_id": m.EventId,
					"trace_id": bridgeMsg.TraceId,
				}).Debug("message expired")
			}
		} else {
			results = append(results, m)
		}
	}
	return results
}

func (s *MemStorage) watcher() {
	for {
		s.lock.Lock()
		for key, msgs := range s.db {
			s.db[key] = removeExpiredMessages(msgs, time.Now(), key)
		}
		s.lock.Unlock()

		_ = ExpiredCache.Cleanup()
		_ = TransferedCache.Cleanup()

		time.Sleep(time.Second)
	}
}

func (s *MemStorage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()
	results := make([]models.SseMessage, 0)
	for _, key := range keys {
		messages, ok := s.db[key]
		if !ok {
			continue
		}
		for _, m := range messages {
			if m.IsExpired(now) {
				continue
			}
			if m.EventId <= lastEventId {
				continue
			}
			results = append(results, m.SseMessage)
		}
	}
	return results, nil
}

func (s *MemStorage) Add(ctx context.Context, mes models.SseMessage, ttl int64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.db[mes.To] = append(s.db[mes.To], message{SseMessage: mes, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)})
	return nil
}

// HealthCheck should be implemented
func (s *MemStorage) HealthCheck() error {
	return nil // Always healthy
}
