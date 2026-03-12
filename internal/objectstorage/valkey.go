package objectstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// keyPrefix is the Redis key namespace for all stored objects.
const keyPrefix = "objstore:"

// valkeyEntry is the JSON-serialized format for objects stored in Valkey/Redis.
type valkeyEntry struct {
	Object      []byte `json:"object"`
	ContentType string `json:"content_type"`
}

// ValkeyObjectStorage is a Valkey/Redis-backed ObjectStorage implementation.
type ValkeyObjectStorage struct {
	client redis.UniversalClient
}

// NewValkeyObjectStorageWithClient creates a ValkeyObjectStorage reusing an existing Redis client.
func NewValkeyObjectStorageWithClient(client redis.UniversalClient) *ValkeyObjectStorage {
	return &ValkeyObjectStorage{client: client}
}

// NewValkeyObjectStorage creates a new Valkey/Redis-backed storage from a connection URI.
// It pings the server on creation to verify connectivity.
func NewValkeyObjectStorage(uri string) (*ValkeyObjectStorage, error) {
	opts, err := redis.ParseURL(strings.TrimSpace(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	log.Info("Object storage connected to Valkey/Redis")

	return &ValkeyObjectStorage{client: client}, nil
}

// Store serializes the object with its content type and saves it to Valkey with the given TTL.
// Returns a content-addressable ID.
func (s *ValkeyObjectStorage) Store(ctx context.Context, object []byte, contentType string, ttl int64) (string, error) {
	id := hashObject(object, contentType)
	key := keyPrefix + id

	entry := valkeyEntry{
		Object:      object,
		ContentType: contentType,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: %w", err)
	}

	err = s.client.Set(ctx, key, data, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store object: %w", err)
	}

	return id, nil
}

// Get retrieves an object by ID from Valkey, deserializes it, and returns the bytes and content type.
func (s *ValkeyObjectStorage) Get(ctx context.Context, id string) ([]byte, string, error) {
	key := keyPrefix + id
	val, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, "", fmt.Errorf("object not found")
		}
		return nil, "", fmt.Errorf("failed to get object: %w", err)
	}

	var entry valkeyEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal object: %w", err)
	}

	return entry.Object, entry.ContentType, nil
}
