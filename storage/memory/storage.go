package memory

import (
	"context"
	"github.com/tonkeeper/bridge/datatype"
	"sync"
	"time"
)

type Storage struct {
	db map[string][]message
	lock sync.Mutex
}

type message struct {
	datatype.SseMessage
	expireAt time.Time
}

func NewStorage() *Storage  {
	s := Storage{
		db: map[string][]message{},
	}
	go s.watcher()
	return &s
}

func (s *Storage) watcher() {
	for {
		time.Sleep(time.Second)
	}
}

func (s *Storage) GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error) {
return nil, nil
}



func (s *Storage) Add(ctx context.Context, key string, ttl int64, mes datatype.SseMessage) error {
return nil
}

