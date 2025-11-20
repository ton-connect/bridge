package handlerv3

import (
	"context"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/v3/storage"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	storage     storagev3.Storage
	messageCh   chan models.SseMessage
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s storagev3.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		messageCh:   make(chan models.SseMessage, 100),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
	return &session
}

// GetMessages returns the read-only channel for receiving messages
func (s *Session) GetMessages() <-chan models.SseMessage {
	return s.messageCh
}

// Close stops the session and cleans up resources
func (s *Session) Close() {
	log := log.WithField("prefix", "Session.Close")
	s.mux.Lock()
	defer s.mux.Unlock()

	err := s.storage.Unsub(context.Background(), s.ClientIds, s.messageCh)
	if err != nil {
		log.Errorf("failed to unsubscribe from storage: %v", err)
	}

	close(s.Closer)
	close(s.messageCh)
}

// Start begins the session by subscribing to storage
func (s *Session) Start() {
	s.mux.Lock()
	defer s.mux.Unlock()

	err := s.storage.Sub(context.Background(), s.ClientIds, s.lastEventId, s.messageCh)
	if err != nil {
		close(s.messageCh)
		return
	}
}
