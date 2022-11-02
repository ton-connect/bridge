package main

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	SessionId   string
	mux         sync.Mutex
	ClientIds   []string
	MessageCh   chan datatype.SseMessage
	storage     *storage.Storage
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(sessionId string, s *storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		SessionId:   sessionId,
		mux:         sync.Mutex{},
		ClientIds:   clientIds,
		MessageCh:   make(chan datatype.SseMessage, 10),
		storage:     s,
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")

	defer func() {
		close(s.MessageCh)
		log.Info("close session")
	}()

	q, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range q {
		select {
		case <-s.Closer:
			return
		default:
			s.MessageCh <- m
		}
	}
	<-s.Closer
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes datatype.SseMessage) {
	select {
	case <-s.Closer:
		log.Info("close session while add message to queue")
	default:
		s.MessageCh <- mes
	}
}

func (s *Session) Start() {
	go s.worker()
}
