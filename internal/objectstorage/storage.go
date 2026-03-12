package objectstorage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ObjectStorage defines the interface for storing and retrieving binary objects with TTL.
type ObjectStorage interface {
	// Store persists the object with the given content type and TTL (in seconds).
	// Returns a content-addressable ID (SHA-256 of object + content type).
	Store(ctx context.Context, object []byte, contentType string, ttl int64) (string, error)
	// Get retrieves a previously stored object by ID.
	// Returns the object bytes, its content type, or an error if not found or expired.
	Get(ctx context.Context, id string) (object []byte, contentType string, err error)
}

// NewObjectStorage creates an ObjectStorage backend based on storageType ("valkey"/"redis" or "memory").
func NewObjectStorage(storageType string, uri string) (ObjectStorage, error) {
	switch storageType {
	case "valkey", "redis":
		return NewValkeyObjectStorage(uri)
	case "memory":
		return NewMemObjectStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported object storage type: %s", storageType)
	}
}

// NewObjectStorageWithClient creates a Valkey-backed ObjectStorage reusing an existing Redis client.
func NewObjectStorageWithClient(client redis.UniversalClient) ObjectStorage {
	return &ValkeyObjectStorage{client: client}
}
