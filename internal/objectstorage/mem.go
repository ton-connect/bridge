package objectstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// storedObject holds an object's data and expiration metadata in the in-memory store.
type storedObject struct {
	Object      []byte
	ContentType string
	ExpiresAt   time.Time
}

// MemObjectStorage is an in-memory ObjectStorage implementation with automatic expiration cleanup.
type MemObjectStorage struct {
	mu      sync.Mutex
	objects map[string]storedObject
}

// NewMemObjectStorage creates a new in-memory storage and starts a background goroutine
// that periodically removes expired objects.
func NewMemObjectStorage() *MemObjectStorage {
	s := &MemObjectStorage{
		objects: make(map[string]storedObject),
	}
	go s.watcher()
	return s
}

// Store saves the object in memory with a TTL. Returns a content-addressable ID.
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

// Get retrieves an object by ID. Returns an error if the object is not found or has expired.
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

// watcher runs in a background goroutine and deletes expired objects every second.
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

// hashObject computes a SHA-256 hash of the object bytes and content type to produce
// a deterministic, content-addressable ID. Frontend uses the same hash function to produce url
// for getting the object, without waiting backend response. DO NOT CHANGE THE HASH FUNCTION.
func hashObject(object []byte, contentType string) string {
	h := sha256.New()
	h.Write(object)
	h.Write([]byte(contentType))
	return hex.EncodeToString(h.Sum(nil))
}
