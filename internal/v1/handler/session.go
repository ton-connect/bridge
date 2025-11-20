package handlerv1

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/v1/storage"
)

type Session struct {
	mux         sync.RWMutex
	ClientIds   []string
	MessageCh   chan models.SseMessage
	storage     storage.Storage
	Closer      chan interface{}
	lastEventId int64
}

func NewSession(s storage.Storage, clientIds []string, lastEventId int64) *Session {
	session := Session{
		mux:         sync.RWMutex{},
		ClientIds:   clientIds,
		storage:     s,
		MessageCh:   make(chan models.SseMessage, 10),
		Closer:      make(chan interface{}),
		lastEventId: lastEventId,
	}
	return &session
}

func (s *Session) worker(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	log := logrus.WithField("prefix", "Session.worker")

	wg := sync.WaitGroup{}
	s.runHeartbeat(&wg, log, heartbeatMessage, heartbeatInterval)

	s.retrieveHistoricMessages(&wg, log, enableQueueDoneEvent)

	// Wait for closer to be closed from the outside
	// Happens when connection is closed
	<-s.Closer

	// Wait for channel producers to finish before closing the channel
	wg.Wait()
	close(s.MessageCh)
}

func (s *Session) runHeartbeat(wg *sync.WaitGroup, log *logrus.Entry, heartbeatMessage string, heartbeatInterval time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.Closer:
				return
			case <-ticker.C:
				s.MessageCh <- models.SseMessage{EventId: -1, Message: []byte(heartbeatMessage)}
			}
		}
	}()
}

func (s *Session) retrieveHistoricMessages(wg *sync.WaitGroup, log *logrus.Entry, doneEvent bool) {
	wg.Add(1)
	defer wg.Done()

	messages, err := s.storage.GetMessages(context.TODO(), s.ClientIds, s.lastEventId)
	if err != nil {
		log.Info("get queue error: ", err)
	}

	for _, m := range messages {
		select {
		case <-s.Closer:
			return
		default:
			s.MessageCh <- m
		}
	}

	if doneEvent {
		s.MessageCh <- models.SseMessage{EventId: -1, Message: []byte("event: message\r\ndata: queue_done\r\n\r\n")}
	}
}

func (s *Session) AddMessageToQueue(ctx context.Context, mes models.SseMessage) {
	select {
	case <-s.Closer:
	default:
		s.MessageCh <- mes
	}
}

func (s *Session) Start(heartbeatMessage string, enableQueueDoneEvent bool, heartbeatInterval time.Duration) {
	go s.worker(heartbeatMessage, enableQueueDoneEvent, heartbeatInterval)
}
