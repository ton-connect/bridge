package objectstorage

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

// getTestValkeyURI returns the Valkey URI from environment or skips the test.
func getTestValkeyURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("VALKEY_URI")
	if uri == "" {
		uri = os.Getenv("REDIS_URI")
	}
	if uri == "" {
		t.Skip("Skipping Valkey integration test: VALKEY_URI or REDIS_URI not set")
	}
	return uri
}

func newTestValkeyStorage(t *testing.T) *ValkeyObjectStorage {
	t.Helper()
	uri := getTestValkeyURI(t)
	s, err := NewValkeyObjectStorage(uri)
	if err != nil {
		t.Fatalf("failed to create ValkeyObjectStorage: %v", err)
	}
	return s
}

func TestValkeyStoreAndGet(t *testing.T) {
	s := newTestValkeyStorage(t)
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

func TestValkeyGetNonExistent(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	_, _, err := s.Get(ctx, "nonexistent-id-that-does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent object")
	}
}

func TestValkeyExpiration(t *testing.T) {
	s := newTestValkeyStorage(t)
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

	// Wait for TTL to expire
	time.Sleep(2 * time.Second)

	_, _, err = s.Get(ctx, id)
	if err == nil {
		t.Fatal("expected error for expired object")
	}
}

func TestValkeyDeduplication(t *testing.T) {
	s := newTestValkeyStorage(t)
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

func TestValkeyDifferentContentDifferentIDs(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	id1, err := s.Store(ctx, []byte("content A"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	id2, err := s.Store(ctx, []byte("content B"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if id1 == id2 {
		t.Fatal("different content should produce different IDs")
	}
}

func TestValkeyDifferentContentTypeDifferentIDs(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	id1, err := s.Store(ctx, []byte("same body"), "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	id2, err := s.Store(ctx, []byte("same body"), "application/json", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if id1 == id2 {
		t.Fatal("same body with different content type should produce different IDs")
	}
}

func TestValkeyContentTypePreserved(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		body        []byte
		contentType string
	}{
		{"text/plain", []byte("hello"), "text/plain"},
		{"application/json", []byte(`{"key":"value"}`), "application/json"},
		{"application/xml", []byte("<root/>"), "application/xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := s.Store(ctx, tt.body, tt.contentType, 60)
			if err != nil {
				t.Fatalf("Store failed: %v", err)
			}

			obj, ct, err := s.Get(ctx, id)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if !bytes.Equal(obj, tt.body) {
				t.Fatalf("expected body %q, got %q", tt.body, obj)
			}
			if ct != tt.contentType {
				t.Fatalf("expected content type %q, got %q", tt.contentType, ct)
			}
		})
	}
}

func TestValkeyOverwriteRefreshesTTL(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	// Store with short TTL
	id, err := s.Store(ctx, []byte("refreshable"), "text/plain", 2)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Re-store same content with fresh TTL
	id2, err := s.Store(ctx, []byte("refreshable"), "text/plain", 60)
	if err != nil {
		t.Fatalf("second Store failed: %v", err)
	}
	if id != id2 {
		t.Fatalf("expected same ID on re-store, got %s and %s", id, id2)
	}

	// Wait past the original TTL
	time.Sleep(2 * time.Second)

	// Should still be available because the second Store refreshed the TTL
	obj, _, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed after TTL refresh: %v", err)
	}
	if !bytes.Equal(obj, []byte("refreshable")) {
		t.Fatalf("expected 'refreshable', got '%s'", obj)
	}
}

func TestValkeyLargeObject(t *testing.T) {
	s := newTestValkeyStorage(t)
	ctx := context.Background()

	// Store a 50KB object
	large := bytes.Repeat([]byte("x"), 50*1024)
	id, err := s.Store(ctx, large, "text/plain", 60)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	obj, ct, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(obj, large) {
		t.Fatalf("retrieved object size %d does not match stored size %d", len(obj), len(large))
	}
	if ct != "text/plain" {
		t.Fatalf("expected content type 'text/plain', got '%s'", ct)
	}
}
