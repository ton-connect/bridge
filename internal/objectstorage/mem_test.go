package objectstorage

import (
	"context"
	"testing"
	"time"
)

func TestMemStoreAndGet(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id, err := s.Store(ctx, "hello world", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if id == "" {
		t.Fatal("Store returned empty ID")
	}

	obj, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if obj != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", obj)
	}
}

func TestMemGetNonExistent(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	_, err := s.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent object")
	}
}

func TestMemExpiration(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id, err := s.Store(ctx, "expiring", 1)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Should exist immediately
	obj, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed immediately after store: %v", err)
	}
	if obj != "expiring" {
		t.Fatalf("expected 'expiring', got '%s'", obj)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	_, err = s.Get(ctx, id)
	if err == nil {
		t.Fatal("expected error for expired object")
	}
}

func TestMemDeduplication(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id1, err := s.Store(ctx, "same content", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	id2, err := s.Store(ctx, "same content", 60)
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

	id1, _ := s.Store(ctx, "content A", 60)
	id2, _ := s.Store(ctx, "content B", 60)

	if id1 == id2 {
		t.Fatal("different content should produce different IDs")
	}
}
