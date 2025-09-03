package main

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
)

type Session struct {
	mux             sync.RWMutex
	ClientIds       []string
	MessageCh       chan datatype.SseMessage
	storage         db
	Closer          chan interface{}
	lastEventId     int64
	connectSourceIP string
}

func NewSession(s db, clientIds []string, lastEventId int64, connectSource string) *Session {
	session := Session{
		mux:             sync.RWMutex{},
		ClientIds:       clientIds,
		storage:         s,
		MessageCh:       make(chan datatype.SseMessage, 10),
		Closer:          make(chan interface{}),
		lastEventId:     lastEventId,
		connectSourceIP: connectSource,
	}
	return &session
}

func (s *Session) worker() {
	log := logrus.WithField("prefix", "Session.worker")
	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range queue {
		// Modify the message to include connect source
		var bridgeMsg datatype.BridgeMessage
		if err := json.Unmarshal(m.Message, &bridgeMsg); err == nil {
			bridgeMsg.BridgeConnectSource = s.connectSourceIP

			if modifiedMessage, err := json.Marshal(bridgeMsg); err == nil {
				m.Message = modifiedMessage
			}
		}

		select {
		case <-s.Closer:
			break //nolint:staticcheck // TODO review golangci-lint issue
		default:
			s.MessageCh <- m
		}
	}

	<-s.Closer
	close(s.MessageCh)
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes datatype.SseMessage) {
	// Modify the message to include connect source
	var bridgeMsg datatype.BridgeMessage
	if err := json.Unmarshal(mes.Message, &bridgeMsg); err == nil {
		bridgeMsg.BridgeConnectSource = s.connectSourceIP

		if modifiedMessage, err := json.Marshal(bridgeMsg); err == nil {
			mes.Message = modifiedMessage
		}
	}

	select {
	case <-s.Closer:
	default:
		s.MessageCh <- mes
	}
}

func (s *Session) Start() {
	go s.worker()
}
