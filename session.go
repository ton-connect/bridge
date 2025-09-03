package main

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan datatype.SseMessage
	storage     db
	Closer      chan interface{}
	histMsg     chan bool
	lastEventId int64
}

func NewSession(s db, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan datatype.SseMessage, 10),
		Closer:      make(chan interface{}),
		histMsg:     make(chan bool),
		lastEventId: lastEventId,
	}
	return &session
}

func (s *Session) worker() {
	log := logrus.WithField("prefix", "Session.worker")
	messages, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range messages {
		select {
		case <-s.Closer:
			break //nolint:staticcheck // TODO review golangci-lint issue
		default:
			s.MessageCh <- m
		}
	}
	s.histMsg <- true
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

func (s *Session) Start() chan bool {
	go s.worker()
	return s.histMsg
}
