package storage

import (
	"context"
	"time"

	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/models"
	common_storage "github.com/ton-connect/bridge/internal/storage"
)

var (
	ExpiredCache    = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)
)

type Storage interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]models.SseMessage, error)
	Add(ctx context.Context, mes models.SseMessage, ttl int64) error
	HealthCheck() error
}

func NewStorage(dbURI string, collector analytics.EventCollector, builder analytics.EventBuilder) (Storage, error) {
	if dbURI != "" {
		return NewPgStorage(dbURI, collector, builder)
	}
	return NewMemStorage(collector, builder), nil
}
