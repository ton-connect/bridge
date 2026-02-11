package objectstorage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type ObjectStorage interface {
	Store(ctx context.Context, object string, ttl int64) (string, error) // returns ID
	Get(ctx context.Context, id string) (string, error)                  // returns object
}

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

func NewObjectStorageWithClient(client redis.UniversalClient) ObjectStorage {
	return &ValkeyObjectStorage{client: client}
}
