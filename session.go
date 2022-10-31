package main

import (
	"context"
	"sync"

	"github.com/gammazero/deque"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	mux       sync.Mutex
	ClientIds []string
	MessageCh chan []byte
	storage   *storage.Storage
	Closer    chan interface{}
	queue     *deque.Deque[[]byte]
}

func NewSession(s *storage.Storage, clientIds []string) *Session {
	session := Session{
		mux:       sync.Mutex{},
		ClientIds: clientIds,
		MessageCh: make(chan []byte, 1),
		storage:   s,
		Closer:    make(chan interface{}, 1),
		queue:     deque.New[[]byte](),
	}

	go session.worker()

	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	q, err := s.storage.GetQueue(context.TODO(), s.ClientIds)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for q.Len() != 0 {
		m := q.PopFront()
		s.queue.PushBack(m)
	}
	for {
		select {
		case <-s.Closer:
			log.Info("close session")
			close(s.MessageCh)
			return
		default:
			s.mux.Lock()
			for s.queue.Len() != 0 {
				s.MessageCh <- s.queue.PopFront()
			}
			s.mux.Unlock()
		}
	}
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes []byte) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.queue.PushBack(mes)
}
