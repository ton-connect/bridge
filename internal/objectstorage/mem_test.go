package objectstorage

import (
	"context"
	"testing"
	"time"
)

func TestMemStoreAndGet(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	id, err := s.Store(ctx, "dGVzdCBvYmplY3Q=", 60)
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
	if obj != "dGVzdCBvYmplY3Q=" {
		t.Fatalf("expected 'dGVzdCBvYmplY3Q=', got '%s'", obj)
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

func TestMemUniqueIDs(t *testing.T) {
	s := NewMemObjectStorage()
	ctx := context.Background()

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := s.Store(ctx, "obj", 60)
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}
