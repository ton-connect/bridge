package handlerv3

import (
	"context"
	"log/slog"
	"sync"

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
	closed      bool
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

// Close stops the session and cleans up resources. Safe to call multiple times.
func (s *Session) Close() {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.closeLocked()
}

// Start begins the session by subscribing to storage. A session closed before it starts, as
// one whose client left during the handshake is, stays unsubscribed.
func (s *Session) Start() {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.closed {
		return
	}
	err := s.storage.Sub(context.Background(), s.ClientIds, s.lastEventId, s.messageCh)
	if err != nil {
		s.closeLocked()
	}
}

// closeLocked unsubscribes before closing messageCh, so no storage still holds the channel.
func (s *Session) closeLocked() {
	if s.closed {
		return
	}
	s.closed = true

	logger := slog.With("prefix", "Session.Close")
	err := s.storage.Unsub(context.Background(), s.ClientIds, s.messageCh)
	if err != nil {
		logger.Error("failed to unsubscribe from storage", "err", err)
	}

	close(s.Closer)
	close(s.messageCh)
}
