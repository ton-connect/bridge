package storage

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	"github.com/gammazero/deque"
)

var expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
	Name: "number_of_expired_messages",
	Help: "The total number of expired messages",
})

type MessageWithTtl struct {
	EndTime time.Time
	Message interface{}
}

type Storage struct {
	mux    sync.Mutex
	queues map[string]*deque.Deque[MessageWithTtl]
}

func NewStorage() *Storage {
	s := Storage{
		mux:    sync.Mutex{},
		queues: make(map[string]*deque.Deque[MessageWithTtl]),
	}
	go s.worker()
	return &s
}

func (s *Storage) worker() {
	log := log.WithField("prefix", "Storage.worker")

	nextCheck := time.Now().Add(time.Minute)
	for {
		if time.Now().After(nextCheck) {
			s.mux.Lock()
			for k, v := range s.queues {
				if v.Len() != 0 {
					a := v.At(0)
					if time.Now().After(a.EndTime) {
						log.Infof("message fo id: %v expired. remove", k)
						expiredMessagesMetric.Inc()
						_ = v.PopFront()
					}
				} else {
					delete(s.queues, k)
				}
			}
			nextCheck = time.Now().Add(time.Minute)
			s.mux.Unlock()
		}
	}
}

func (s *Storage) Add(key string, ttl int64, value interface{}) {
	s.mux.Lock()
	defer s.mux.Unlock()
	queue, ok := s.queues[key]
	if !ok {
		queue = deque.New[MessageWithTtl]()
		s.queues[key] = queue
	}
	queue.PushBack(MessageWithTtl{
		EndTime: time.Now().Add(time.Duration(ttl) * time.Second),
		Message: value,
	})
}

func (s *Storage) GetQueue(keys []string) *deque.Deque[interface{}] {
	log := log.WithField("prefix", "Storage.GetQueue")
	s.mux.Lock()
	defer s.mux.Unlock()
	sessionQueue := deque.New[interface{}]()
	for _, key := range keys {
		log.Infof("check id: %v", key)
		queue, ok := s.queues[key]
		if !ok {
			continue
		}
		for queue.Len() != 0 {
			m := queue.PopFront()
			if !time.Now().After(m.EndTime) {
				sessionQueue.PushBack(m.Message)
			}
		}
	}
	return sessionQueue
}
