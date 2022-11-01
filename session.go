package main

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	mux       sync.Mutex
	ClientIds []string
	MessageCh chan []byte
	storage   *storage.Storage
	Closer    chan interface{}
	queue     [][]byte
}

func NewSession(s *storage.Storage, clientIds []string) *Session {
	session := Session{
		mux:       sync.Mutex{},
		ClientIds: clientIds,
		MessageCh: make(chan []byte, 1),
		storage:   s,
		Closer:    make(chan interface{}, 1),
		queue:     make([][]byte, 0),
	}

	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	q, err := s.storage.GetMessages(context.TODO(), s.ClientIds)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	s.queue = append(s.queue, q...)
	for {
		select {
		case <-s.Closer:
			log.Info("close session")
			close(s.MessageCh)
			return
		default:
			s.mux.Lock()
			if len(s.queue) != 0 {
				for _, m := range s.queue {
					s.MessageCh <- m
				}
				s.queue = s.queue[:0]
			}
			s.mux.Unlock()
		}
	}
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes []byte) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.queue = append(s.queue, mes)
}

func (s *Session) Start() {
	go s.worker()
}
