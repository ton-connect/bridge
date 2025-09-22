package storage

import (
	"context"
	"time"

	"github.com/tonkeeper/bridge/datatype"
	common_storage "github.com/tonkeeper/bridge/internal/storage"
)

var (
	// ExpiredCache    = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	// TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)
	ExpiredCache    = common_storage.NewMessageCache(time.Hour)
	TransferedCache = common_storage.NewMessageCache(time.Minute)
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error)
	Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error
	// HealthCheck() error
}

func NewStorage(dbURI string) (Storage, error) {
	if dbURI != "" {
		return NewPgStorage(dbURI)
	}
	return NewMemStorage(), nil
}
