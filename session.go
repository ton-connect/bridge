package main

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/tonkeeper/bridge/storage"
)

type Session struct {
	mux       sync.Mutex
	ClientIds []string
	MessageCh chan BridgeMessage
	storage   *storage.Storage[storage.MessageWithTtl]
	Closer    chan interface{}
}

func NewSession(s *storage.Storage[storage.MessageWithTtl], clientIds []string) *Session {
	session := Session{
		mux:       sync.Mutex{},
		ClientIds: clientIds,
		MessageCh: make(chan BridgeMessage, 1),
		storage:   s,
		Closer:    make(chan interface{}, 1),
	}

	go session.worker()

	return &session
}

func (s *Session) worker() {
	log := log.WithField("prefix", "Session.worker")
	for {
		s.mux.Lock()
		ids := s.ClientIds
		s.mux.Unlock()
		select {
		case <-s.Closer:
			log.Info("close session")
			close(s.MessageCh)
			return
		default:
			for _, id := range ids {
				v, err := s.storage.Get(id)
				if err != nil {
					continue
				}
				if v == nil {
					continue
				}
				select {
				case <-v.RemoveMessage:
					log.Info("request closed. remove message")
				default:
					s.MessageCh <- BridgeMessage{
						From:    v.From,
						Message: v.Message,
					}
					v.RequestCloser <- true
				}
			}
		}

	}
}
