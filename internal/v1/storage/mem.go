package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

type MemStorage struct {
	db   map[string][]message
	lock sync.Mutex
}

type message struct {
	datatype.SseMessage
	expireAt time.Time
}

var removeExpiredLogger = log.WithField("prefix", "removeExpiredMessages")

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

func removeExpiredMessages(messages []message, now time.Time, clientID string) []message {
	i := 0
	for _, msg := range messages {
		if !msg.IsExpired(now) {
			messages[i] = msg
			i++
			continue
		}

		if !ExpiredCache.IsMarked(msg.EventId) &&
			removeExpiredLogger.Logger != nil &&
			removeExpiredLogger.Logger.IsLevelEnabled(log.DebugLevel) {

			var (
				bridgeMsg   datatype.BridgeMessage
				fromID      = "unknown"
				messageHash string
			)
			if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
				fromID = bridgeMsg.From
				contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
				messageHash = hex.EncodeToString(contentHash[:])
			} else {
				sum := sha256.Sum256(msg.Message)
				messageHash = hex.EncodeToString(sum[:])
			}

			removeExpiredLogger.WithFields(log.Fields{
				"hash":     messageHash,
				"from":     fromID,
				"to":       clientID,
				"event_id": msg.EventId,
				"trace_id": bridgeMsg.TraceId,
			}).Debug("message expired")
		}
	}

	for j := i; j < len(messages); j++ {
		messages[j] = message{}
	}
	return messages[:i]
}

func (s *MemStorage) watcher() {
	for {
		s.lock.Lock()
		for key, msgs := range s.db {
			s.db[key] = removeExpiredMessages(msgs, time.Now(), key)
		}
		s.lock.Unlock()

		_ = ExpiredCache.Cleanup()
		_ = TransferredCache.Cleanup()

		time.Sleep(time.Second)
	}
}

func (s *MemStorage) GetMessages(_ context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error) {
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

func (s *MemStorage) Add(_ context.Context, mes datatype.SseMessage, ttl int64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.db[mes.To] = append(s.db[mes.To], message{SseMessage: mes, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)})
	return nil
}
