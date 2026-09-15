package storagev3

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/models"
	common_storage "github.com/ton-connect/bridge/internal/storage"
)

var (
	ExpiredCache = common_storage.NewMessageCache(config.Config.EnableExpiredCache, time.Hour)
	// TransferedCache = common_storage.NewMessageCache(config.Config.EnableTransferedCache, time.Minute)

	// Every backend in this package reports into this counter, which is why it lives here
	// rather than beside one of them. It used to be declared in mem.go, and that placement
	// was not harmless: promauto registers a counter as soon as the package is imported, so
	// a binary running STORAGE=valkey published number_of_expired_messages and held it at
	// zero forever, the only increment sitting on the memory path. A permanent zero on a
	// loss counter reads as "nothing expired" and means "nothing is counting".
	expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_expired_messages",
		Help: "The total number of expired messages",
	})
)

// deliveredMarkTTL bounds how long a delivery mark is kept. It only has to outlive the
// message it describes, and message TTLs are minutes, so an hour is generous. It matches
// the TTL of the in-process cache the memory backend uses for the same purpose.
const deliveredMarkTTL = time.Hour

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

	// MarkDelivered records that a message reached a subscriber, so the expiry sweep can
	// tell a message nobody ever received from a backup copy of one already delivered.
	// Both live in the backlog until their TTL: Pub stores every message whether or not a
	// subscriber was listening, and a delivered one is only removed when it expires.
	MarkDelivered(ctx context.Context, clientID string, eventID int64) error

	// Connection verification methods
	AddConnection(ctx context.Context, conn ConnectionInfo, ttl time.Duration) error
	VerifyConnection(ctx context.Context, conn ConnectionInfo) (string, error)

	HealthCheck() error
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
