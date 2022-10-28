package main

import (
	"sync"

	"github.com/gammazero/deque"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	mux       sync.Mutex
	ClientIds []string
	MessageCh chan BridgeMessage
	storage   *storage.Storage
	Closer    chan interface{}
	queue     *deque.Deque[BridgeMessage]
}

func NewSession(s *storage.Storage, clientIds []string) *Session {
	session := Session{
		mux:       sync.Mutex{},
		ClientIds: clientIds,
		MessageCh: make(chan BridgeMessage, 1),
		storage:   s,
		Closer:    make(chan interface{}, 1),
		queue:     deque.New[BridgeMessage](),
	}

	go session.worker()

	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	q := s.storage.GetQueue(s.ClientIds)
	for q.Len() != 0 {
		m, ok := q.PopFront().(BridgeMessage)
		if !ok {
			continue
		}
		s.queue.PushBack(m)
	}
	for {
		s.mux.Lock()
		select {
		case <-s.Closer:
			log.Info("close session")
			close(s.MessageCh)
			return
		default:
			for s.queue.Len() != 0 {
				s.MessageCh <- s.queue.PopFront()
			}

		}
		s.mux.Unlock()
	}
}

func (s *Session) AddMessageToQueue(mes BridgeMessage) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.queue.PushBack(mes)

}
