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
	ctx         context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan datatype.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
		ctx:         ctx,
		cancel:      cancel,
		closeOnce:   sync.Once{},
	}
	return &session
}

func (s *Session) worker(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	log := logrus.WithField("prefix", "Session.worker")

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.Closer:
				return
			case <-ticker.C:
				s.AddMessageToQueue(datatype.SseMessage{EventId: -1, Message: []byte(heartbeatMessage)})
			}
		}
	}()

	s.retrieveHistoricMessages(log, enableQueueDoneEvent)

	<-s.Closer
	s.closeSession()
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
	case <-s.ctx.Done():
		return false
	default:
	}

	select {
	case <-s.ctx.Done():
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

// closeSession safely closes the MessageCh channel
func (s *Session) closeSession() {
	s.closeOnce.Do(func() {
		s.cancel()         // Cancel the context first to stop all goroutines
		s.wg.Wait()        // Wait for all goroutines to finish
		close(s.MessageCh) // Now it's safe to close the channel
	})
}
