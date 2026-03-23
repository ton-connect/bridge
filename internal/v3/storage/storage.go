package storagev3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/models"
	common_storage "github.com/ton-connect/bridge/internal/storage"
)

var (
	ExpiredCache = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	// TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)
)

// ConnectionInfo represents connection metadata for verification
type ConnectionInfo struct {
	ClientID  string
	IP        string
	Origin    string
	UserAgent string
}

type Storage interface {
	Pub(ctx context.Context, message models.SseMessage, ttl int64) error
	Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error
	Unsub(ctx context.Context, keys []string, messageCh chan<- models.SseMessage) error

	// Connection verification methods
	AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error
	VerifyConnection(ctx context.Context, conn ConnectionInfo) (string, error)

	// Object storage methods
	StoreObject(ctx context.Context, object []byte, contentType string, ttl int64) (string, error)
	GetObject(ctx context.Context, id string) (object []byte, contentType string, err error)

	HealthCheck() error
}

// HashObject computes a SHA-256 hash of the object bytes and content type to produce
// a deterministic, content-addressable ID. Frontend uses the same hash function to produce url
// for getting the object, without waiting backend response. DO NOT CHANGE THE HASH FUNCTION.
func HashObject(object []byte, contentType string) string {
	h := sha256.New()
	h.Write(object)
	h.Write([]byte(contentType))
	return hex.EncodeToString(h.Sum(nil))
}

func NewStorage(storageType string, uri string, collector analytics.EventCollector, builder analytics.EventBuilder) (Storage, error) {
	switch storageType {
	case "valkey", "redis":
		return NewValkeyStorage(uri)
	case "postgres":
		return nil, fmt.Errorf("postgres storage does not support pub-sub functionality yet")
	case "memory":
		return NewMemStorage(collector, builder), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
