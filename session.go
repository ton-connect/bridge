package main

import (
	"encoding/base64"
	"sync"
	"time"

	"github.com/gammazero/deque"
	log "github.com/sirupsen/logrus"

	"github.com/labstack/echo/v4"
)

type Session struct {
	mux           sync.Mutex
	SessionId     string
	Connection    *echo.Context
	MessageCh     chan BridgeMessage
	MessageQueue  *deque.Deque[MessageWithTtl]
	SessionCloser chan interface{}
	Subscribers   int
}

type MessageWithTtl struct {
	PushTime      time.Time
	Ttl           int64
	From          string
	Message       []byte
	RequestCloser chan interface{}
}

func NewSession(sessionId string, c *echo.Context, subscribers int) *Session {
	session := Session{
		mux:           sync.Mutex{},
		SessionId:     sessionId,
		Connection:    c,
		MessageCh:     make(chan BridgeMessage),
		MessageQueue:  deque.New[MessageWithTtl](),
		SessionCloser: make(chan interface{}),
		Subscribers:   subscribers,
	}

	go session.worker()

	return &session
}

func (s *Session) addMessageToDeque(from string, ttl int64, message []byte, done chan interface{}) {
	s.mux.Lock()
	s.MessageQueue.PushBack(MessageWithTtl{
		PushTime:      time.Now(),
		Ttl:           ttl,
		From:          from,
		Message:       message,
		RequestCloser: done,
	})
	s.mux.Unlock()
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	for {
		select {
		case <-s.SessionCloser:
			log.Info("close session. remove from worker")
			return
		default:
			if s.MessageQueue.Len() > 0 {
				s.mux.Lock()
				mes := s.MessageQueue.PopFront()
				s.mux.Unlock()
				if time.Now().After(mes.PushTime.Add(time.Duration(mes.Ttl) * time.Second)) {
					log.Info("timeout message")
					continue
				}
				s.MessageCh <- BridgeMessage{
					From:    base64.RawStdEncoding.EncodeToString([]byte(mes.From)),
					Message: base64.RawStdEncoding.EncodeToString(mes.Message),
				}
				mes.RequestCloser <- true
			}

		}
	}
}
