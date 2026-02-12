package objectstorage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const keyPrefix = "objstore:"

type ValkeyObjectStorage struct {
	client redis.UniversalClient
}

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

func (s *ValkeyObjectStorage) Store(ctx context.Context, object string, ttl int64) (string, error) {
	id := hashObject(object)

	key := keyPrefix + id
	err := s.client.Set(ctx, key, object, time.Duration(ttl)*time.Second).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store object: %w", err)
	}

	return id, nil
}

func (s *ValkeyObjectStorage) Get(ctx context.Context, id string) (string, error) {
	key := keyPrefix + id
	val, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("object not found")
		}
		return "", fmt.Errorf("failed to get object: %w", err)
	}

	return val, nil
}
