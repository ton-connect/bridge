package storagev3

import (
	"context"
	"fmt"
	"time"

	"github.com/tonkeeper/bridge/internal/config"
	"github.com/tonkeeper/bridge/internal/models"
	common_storage "github.com/tonkeeper/bridge/internal/storage"
)

var (
	ExpiredCache = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	// TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)
)

type Storage interface {
    Pub(ctx context.Context, message models.SseMessage, ttl int64) error
    Sub(ctx context.Context, keys []string, lastEventId int64, messageCh chan<- models.SseMessage) error
    Unsub(ctx context.Context, keys []string, messageCh chan<- models.SseMessage) error
    HealthCheck() error
}

func NewStorage(storageType string, uri string) (Storage, error) {
	switch storageType {
	case "valkey", "redis":
		return NewValkeyStorage(uri)
	case "postgres":
		return nil, fmt.Errorf("postgres storage does not support pub-sub functionality yet")
	case "memory":
		return NewMemStorage(), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
