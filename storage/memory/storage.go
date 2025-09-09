package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/storage"
)

type Storage struct {
	db   map[string][]message
	lock sync.Mutex
}

type message struct {
	datatype.SseMessage
	expireAt time.Time
}

func (m message) IsExpired(now time.Time) bool {
	return m.expireAt.Before(now)
}

func NewStorage() *Storage {
	s := Storage{
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
			if !storage.ExpiredCache.IsMarked(m.EventId) {
				var bridgeMsg datatype.BridgeMessage
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

func (s *Storage) watcher() {
	for {
		s.lock.Lock()
		for key, msgs := range s.db {
			s.db[key] = removeExpiredMessages(msgs, time.Now(), key)
		}
		s.lock.Unlock()

		_ = storage.ExpiredCache.Cleanup()
		_ = storage.TransferedCache.Cleanup()

		time.Sleep(time.Second)
	}
}

func (s *Storage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	now := time.Now()
	results := make([]datatype.SseMessage, 0)
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

func (s *Storage) Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.db[mes.To] = append(s.db[mes.To], message{SseMessage: mes, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)})
	return nil
}
