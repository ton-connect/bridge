package main

import (
	"strings"
	"sync"
	"time"

	"github.com/gammazero/deque"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/labstack/echo/v4"
)

type Session struct {
	mux           sync.Mutex
	ClientIds     string
	SessionId     uuid.UUID
	Connection    *echo.Context
	MessageCh     chan BridgeMessage
	MessageQueue  *deque.Deque[MessageWithTtl]
	RemoveSession chan SessionChan
	Subscribers   int
}

type SessionChan struct {
	Ids       []string
	SessionId uuid.UUID
}

type MessageWithTtl struct {
	PushTime      time.Time
	Ttl           int64
	From          string
	To            string
	Message       []byte
	RequestCloser chan interface{}
	RemoveMessage chan interface{}
}

func NewSession(clientIds string, c *echo.Context, subscribers int, remover chan SessionChan) *Session {
	session := Session{
		mux:           sync.Mutex{},
		ClientIds:     clientIds,
		SessionId:     uuid.NewV4(),
		Connection:    c,
		MessageCh:     make(chan BridgeMessage),
		MessageQueue:  deque.New[MessageWithTtl](),
		RemoveSession: remover,
		Subscribers:   subscribers,
	}

	go session.worker()

	return &session
}

func (s *Session) addMessageToDeque(from, to string, ttl int64, message []byte, done, remove chan interface{}) {
	s.mux.Lock()
	s.MessageQueue.PushBack(MessageWithTtl{
		PushTime:      time.Now(),
		Ttl:           ttl,
		From:          from,
		To:            to,
		Message:       message,
		RequestCloser: done,
		RemoveMessage: remove,
	})
	s.mux.Unlock()
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	lastCheckTime := time.Now()
	for {
		if s.Connection != nil {
			if s.MessageQueue.Len() > 0 {
				s.mux.Lock()
				mes := s.MessageQueue.PopFront()
				s.mux.Unlock()
				select {
				case <-mes.RemoveMessage:
					log.Info("request closed. remove message from queue")
					continue
				default:
					if time.Now().After(mes.PushTime.Add(time.Duration(mes.Ttl) * time.Second)) {
						log.Info("timeout message")
						continue
					}
					s.MessageCh <- BridgeMessage{
						From:    mes.From,
						Message: string(mes.Message),
					}
					mes.RequestCloser <- true
				}
			}
		} else {
			if time.Now().After(lastCheckTime.Add(time.Second)) {
				lastCheckTime = time.Now()
				s.mux.Lock()
				if s.MessageQueue.Len() == 0 {
					log.Infof("message queue is empty. remove from worker sessionId: %v", s.SessionId.String())
					close(s.MessageCh)
					s.RemoveSession <- SessionChan{
						Ids:       strings.Split(s.ClientIds, ","),
						SessionId: s.SessionId,
					}
					return
				}
				m := s.MessageQueue.At(0)
				select {
				case <-m.RemoveMessage:
					log.Info("request closed. remove message from queue")
					_ = s.MessageQueue.PopFront()
				default:
					if time.Now().After(m.PushTime.Add(time.Duration(m.Ttl) * time.Second)) {
						log.Info("remove timeout message")

						_ = s.MessageQueue.PopFront()
					}
				}

				s.mux.Unlock()
			}
		}

	}
}
