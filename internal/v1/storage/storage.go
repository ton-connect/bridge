package storage

import (
	"context"

	"github.com/tonkeeper/bridge/datatype"
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
