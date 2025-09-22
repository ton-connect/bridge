package handlerv1

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/internal/v1/storage"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan datatype.SseMessage
	storage     storage.Storage
	Closer      chan interface{}
	lastEventId int64
	done        chan struct{}
}

func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan datatype.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
		done:        make(chan struct{}),
	}
	return &session
}

func (s *Session) worker(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	log := logrus.WithField("prefix", "Session.worker")

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	go func() {
		defer close(s.done) // Signal when this goroutine is done
		for {
			select {
			case <-s.Closer:
				return
			case <-ticker.C:
				s.AddMessageToQueue(datatype.SseMessage{EventId: -1, Message: []byte(heartbeatMessage)})
			}
		}
	}()

	s.retrieveHistoricMessages(log, enableQueueDoneEvent)

	<-s.Closer
	<-s.done // Wait for the heartbeat goroutine to finish
	close(s.MessageCh)
}

func (s *Session) retrieveHistoricMessages(log *logrus.Entry, doneEvent bool) {
	messages, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}

	for _, m := range messages {
		if !s.AddMessageToQueue(m) {
			return // Session is closed, stop sending
		}
	}

	if doneEvent {
		s.AddMessageToQueue(datatype.SseMessage{EventId: -1, Message: []byte("event: message\r\ndata: queue_done\r\n\r\n")})
	}
}

// AddMessageToQueue safely attempts to add a message to the session's message queue.
func (s *Session) AddMessageToQueue(msg datatype.SseMessage) bool {
	select {
	case <-s.done:
		return false
	case <-s.Closer:
		return false
	case s.MessageCh <- msg:
		return true
	default:
		// Channel is full, could log this or handle differently
		return false
	}
}

func (s *Session) Start(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	go s.worker(heartbeatMessage, enableQueueDoneEvent, heartbeatInterval)
}
