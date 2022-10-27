package storage

import (
	"reflect"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gammazero/deque"
)

type MessageWithTtl struct {
	PushTime      time.Time
	Ttl           int64
	From          string
	To            string
	Message       []byte
	RequestCloser chan interface{}
	RemoveMessage chan interface{}
}

type Storage[T any] struct {
	mux    sync.Mutex
	queues map[string]*deque.Deque[T]
	hasTtl bool
}

func NewStorage[T any]() *Storage[T] {
	v := new(T)
	metaT := reflect.ValueOf(v).Elem()
	field := metaT.FieldByName("Ttl")
	hasTtl := false
	if field.CanSet() {
		hasTtl = true
	}
	s := Storage[T]{
		mux:    sync.Mutex{},
		queues: make(map[string]*deque.Deque[T]),
		hasTtl: hasTtl,
	}
	go s.worker()
	return &s
}

func (s *Storage[T]) checkTtlExp(value T) bool {
	v := reflect.ValueOf(value).Interface().(MessageWithTtl)
	return time.Now().After(v.PushTime.Add(time.Duration(int(v.Ttl)) * time.Second))
}

func (s *Storage[T]) worker() {
	log := log.WithField("prefix", "Storage.worker")

	lastCheck := time.Now()
	for {
		if s.hasTtl {
			if time.Now().After(lastCheck.Add(time.Second)) {
				s.mux.Lock()
				for k, v := range s.queues {
					if v.Len() != 0 {
						if s.checkTtlExp(v.At(0)) {
							log.Info("message expired. remove")
							_ = v.PopFront()
						}
					} else {
						delete(s.queues, k)
					}
				}
				lastCheck = time.Now()
				s.mux.Unlock()
			}

		}

	}
}

func (s *Storage[T]) Add(key string, value T) {
	s.mux.Lock()
	defer s.mux.Unlock()
	queue, ok := s.queues[key]
	if !ok {
		queue = deque.New[T]()
		s.queues[key] = queue
	}
	queue.PushBack(value)
}

func (s *Storage[T]) Get(key string) (*T, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	queue, ok := s.queues[key]
	if !ok {
		queue = deque.New[T]()
		s.queues[key] = queue
	}
	if queue.Len() == 0 {
		return nil, nil //fmt.Errorf("queue is empty")
	}
	val := queue.PopFront()
	return &val, nil
}

func (s *Storage[T]) QueueLen(key string) uint32 {
	s.mux.Lock()
	defer s.mux.Unlock()
	queue, ok := s.queues[key]
	if !ok {
		queue = deque.New[T]()
		s.queues[key] = queue
	}
	return uint32(queue.Len())
}
