package objectstorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type storedObject struct {
	Object    string
	ExpiresAt time.Time
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

func (s *MemObjectStorage) Store(ctx context.Context, object string, ttl int64) (string, error) {
	id, err := generateID()
	if err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects[id] = storedObject{
		Object:    object,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	return id, nil
}

func (s *MemObjectStorage) Get(ctx context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj, exists := s.objects[id]
	if !exists || time.Now().After(obj.ExpiresAt) {
		if exists {
			delete(s.objects, id)
		}
		return "", fmt.Errorf("object not found")
	}

	return obj.Object, nil
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

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
