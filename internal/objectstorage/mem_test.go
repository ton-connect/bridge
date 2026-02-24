package objectstorage

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestMemStoreAndGet(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id, err := s.Store(ctx, []byte("hello world"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if id == "" {
		t.Fatal("Store returned empty ID")
	}

	obj, ct, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(obj, []byte("hello world")) {
		t.Fatalf("expected 'hello world', got '%s'", obj)
	}
	if ct != "text/plain" {
		t.Fatalf("expected content type 'text/plain', got '%s'", ct)
	}
}

func TestMemGetNonExistent(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	_, _, err := s.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent object")
	}
}

func TestMemExpiration(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id, err := s.Store(ctx, []byte("expiring"), "application/json", 1)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should exist immediately
	obj, ct, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed immediately after store: %v", err)
	}
	if !bytes.Equal(obj, []byte("expiring")) {
		t.Fatalf("expected 'expiring', got '%s'", obj)
	}
	if ct != "application/json" {
		t.Fatalf("expected content type 'application/json', got '%s'", ct)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	_, _, err = s.Get(ctx, id)
	if err == nil {
		t.Fatal("expected error for expired object")
	}
}

func TestMemDeduplication(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id1, err := s.Store(ctx, []byte("same content"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	id2, err := s.Store(ctx, []byte("same content"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("same content should produce same ID, got %s and %s", id1, id2)
	}
}

func TestMemDifferentContentDifferentIDs(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id1, _ := s.Store(ctx, []byte("content A"), "text/plain", 60)
	id2, _ := s.Store(ctx, []byte("content B"), "text/plain", 60)

	if id1 == id2 {
		t.Fatal("different content should produce different IDs")
	}
}
