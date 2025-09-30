package storage

import (
	"context"
	"time"

	"github.com/tonkeeper/bridge/config"
	"github.com/tonkeeper/bridge/internal/models"
	common_storage "github.com/tonkeeper/bridge/internal/storage"
)

var (
	ExpiredCache    = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, mes models.SseMessage, ttl int64) error
	// HealthCheck() error
}

func NewStorage(dbURI string) (Storage, error) {
	if dbURI != "" {
		return NewPgStorage(dbURI)
	}
	return NewMemStorage(), nil
}
