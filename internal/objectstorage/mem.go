package objectstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type storedObject struct {
	Object      []byte
	ContentType string
	ExpiresAt   time.Time
}

type MemObjectStorage struct {
	mu      sync.Mutex
	objects map[string]storedObject
}

func NewMemObjectStorage() *MemObjectStorage {
	s := &MemObjectStorage{
		objects: make(map[string]storedObject),
	}
	go s.watcher()
	return s
}

func (s *MemObjectStorage) Store(ctx context.Context, object []byte, contentType string, ttl int64) (string, error) {
	id := hashObject(object, contentType)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects[id] = storedObject{
		Object:      object,
		ContentType: contentType,
		ExpiresAt:   time.Now().Add(time.Duration(ttl) * time.Second),
	}

	return id, nil
}

func (s *MemObjectStorage) Get(ctx context.Context, id string) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj, exists := s.objects[id]
	if !exists || time.Now().After(obj.ExpiresAt) {
		if exists {
			delete(s.objects, id)
		}
		return nil, "", fmt.Errorf("object not found")
	}

	return obj.Object, obj.ContentType, nil
}

func (s *MemObjectStorage) watcher() {
	for {
		time.Sleep(time.Second)
		s.mu.Lock()
		now := time.Now()
		for id, obj := range s.objects {
			if now.After(obj.ExpiresAt) {
				delete(s.objects, id)
			}
		}
		s.mu.Unlock()
	}
}

func hashObject(object []byte, contentType string) string {
	h := sha256.New()
	h.Write(object)
	h.Write([]byte(contentType))
	return hex.EncodeToString(h.Sum(nil))
}
