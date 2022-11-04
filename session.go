package main

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan datatype.SseMessage
	storage     *storage.Storage
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s *storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan datatype.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range queue {
		select {
		case <-s.Closer:
			break
		default:
			s.MessageCh <- m
		}
	}

	<-s.Closer
	close(s.MessageCh)
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes datatype.SseMessage) {
	select {
	case <-s.Closer:
	default:
		s.MessageCh <- mes
	}
}

func (s *Session) Start() {
	go s.worker()
}
