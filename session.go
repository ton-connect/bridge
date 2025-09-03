package main

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan datatype.SseMessage
	storage     db
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s db, clientIds []string, lastEventId int64) *Session {
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

func (s *Session) worker(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	log := logrus.WithField("prefix", "Session.worker")

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			select {
			case <-s.Closer:
				return
			default:
				s.MessageCh <- datatype.SseMessage{EventId: -1, Message: heartbeatMessage}
			}
		}
	}()

	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range queue {
		select {
		case <-s.Closer:
			break //nolint:staticcheck // TODO review golangci-lint issue
		default:
			s.MessageCh <- m
		}
	}

	if enableQueueDoneEvent {
		s.MessageCh <- datatype.SseMessage{EventId: -1, Message: []byte("event: message\r\ndata: queue_done\r\n\r\n")}
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

func (s *Session) Start(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	go s.worker(hearbeatMessage, enableQueueDoneEvent, heartbeatInterval)
}
