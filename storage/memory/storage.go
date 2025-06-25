package memory

import (
	"context"
	"sync"
	"time"

	"github.com/tonkeeper/bridge/datatype"
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

func removeExpiredMessages(ms []message, now time.Time) []message {
	results := make([]message, 0)
	for _, m := range ms {
		if !m.IsExpired(now) {
			results = append(results, m)
		}
	}
	return results
}

func (s *Storage) watcher() {
	for {
		s.lock.Lock()
		for key, ms := range s.db {
			s.db[key] = removeExpiredMessages(ms, time.Now())
		}
		s.lock.Unlock()
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

func (s *Storage) Add(ctx context.Context, key string, ttl int64, mes datatype.SseMessage) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.db[key] = append(s.db[key], message{SseMessage: mes, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)})
	return nil
}
