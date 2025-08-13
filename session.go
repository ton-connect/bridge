package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
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

func (s *Session) worker() {
	log := logrus.WithField("prefix", "Session.worker")
	queue, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}
	for _, m := range queue {
		fromId := "unknown"
		toId := strings.Join(s.ClientIds, ",")

		hash := sha256.Sum256(m.Message)
		messageHash := hex.EncodeToString(hash[:])

		var bridgeMsg datatype.BridgeMessage
		if err := json.Unmarshal(m.Message, &bridgeMsg); err == nil {
			fromId = bridgeMsg.From
			contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
			messageHash = hex.EncodeToString(contentHash[:])
		}

		logrus.WithFields(logrus.Fields{
			"hash":     messageHash,
			"from":     fromId,
			"to":       toId,
			"event_id": m.EventId,
		}).Debug("message received")

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
