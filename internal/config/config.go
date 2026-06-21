package config

import (
	"log/slog"
	"os"

	"github.com/caarlos0/env/v6"
)

var Config = struct {
	// Core Settings
	LogLevel     string `env:"LOG_LEVEL" envDefault:"info"`
	Port         int    `env:"PORT" envDefault:"8081"`
	MetricsPort  int    `env:"METRICS_PORT" envDefault:"9103"`
	PprofEnabled bool   `env:"PPROF_ENABLED" envDefault:"true"`
	// Graceful shutdown. On SIGTERM /readyz flips to 503 first; we keep serving for
	// ShutdownDrainDelay so the load balancer health-check marks the instance unhealthy and stops
	// routing before we stop accepting, then drain in-flight requests for up to ShutdownTimeout. SSE
	// streams never complete on their own, so ShutdownTimeout caps how long we wait on them
	// (clients reconnect). Both must sum below the pod terminationGracePeriodSeconds (40s).
	ShutdownDrainDelay int `env:"SHUTDOWN_DRAIN_DELAY" envDefault:"15"`
	ShutdownTimeout    int `env:"SHUTDOWN_TIMEOUT" envDefault:"10"`

	// Storage
	Storage   string `env:"STORAGE" envDefault:"memory"` // For v3 only: memory or valkey
	ValkeyURI string `env:"VALKEY_URI"`

	// PostgreSQL Pool Settings
	PostgresURI                   string `env:"POSTGRES_URI"`
	PostgresMaxConns              int32  `env:"POSTGRES_MAX_CONNS" envDefault:"25"`
	PostgresMinConns              int32  `env:"POSTGRES_MIN_CONNS" envDefault:"0"`
	PostgresMaxConnLifetime       string `env:"POSTGRES_MAX_CONN_LIFETIME" envDefault:"1h"`
	PostgresMaxConnLifetimeJitter string `env:"POSTGRES_MAX_CONN_LIFETIME_JITTER" envDefault:"10m"`
	PostgresMaxConnIdleTime       string `env:"POSTGRES_MAX_CONN_IDLE_TIME" envDefault:"30m"`
	PostgresHealthCheckPeriod     string `env:"POSTGRES_HEALTH_CHECK_PERIOD" envDefault:"1m"`
	PostgresLazyConnect           bool   `env:"POSTGRES_LAZY_CONNECT" envDefault:"false"`

	// Performance & Limits
	HeartbeatInterval     int      `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
	RPSLimit              int      `env:"RPS_LIMIT" envDefault:"10"`
	ConnectionsLimit      int      `env:"CONNECTIONS_LIMIT" envDefault:"50"`
	MaxBodySize           int64    `env:"MAX_BODY_SIZE" envDefault:"10485760"`      // 10 MB
	SSEMaxLifetime        int64    `env:"SSE_MAX_LIFETIME" envDefault:"7200"`       // 2 hours in seconds
	SSEMaxLifetimeJitter  int64    `env:"SSE_MAX_LIFETIME_JITTER" envDefault:"900"` // up to 15 min in seconds
	RateLimitsByPassToken []string `env:"RATE_LIMITS_BY_PASS_TOKEN"`

	// Security
	CorsEnable         bool     `env:"CORS_ENABLE" envDefault:"true"`
	TrustedProxyRanges []string `env:"TRUSTED_PROXY_RANGES" envDefault:"0.0.0.0/0"`
	SelfSignedTLS      bool     `env:"SELF_SIGNED_TLS" envDefault:"false"`

	// Caching
	ConnectCacheSize      int  `env:"CONNECT_CACHE_SIZE" envDefault:"2000000"`
	ConnectCacheTTL       int  `env:"CONNECT_CACHE_TTL" envDefault:"300"`
	EnableTransferedCache bool `env:"ENABLE_TRANSFERED_CACHE" envDefault:"true"`
	EnableExpiredCache    bool `env:"ENABLE_EXPIRED_CACHE" envDefault:"true"`

	// Events & Webhooks
	DisconnectEventsTTL    int64  `env:"DISCONNECT_EVENTS_TTL" envDefault:"3600"`
	DisconnectEventMaxSize int    `env:"DISCONNECT_EVENT_MAX_SIZE" envDefault:"512"`
	WebhookURL             string `env:"WEBHOOK_URL"`
	CopyToURL              string `env:"COPY_TO_URL"`

	// NTP Configuration
	NTPEnabled      bool     `env:"NTP_ENABLED" envDefault:"true"`
	NTPServers      []string `env:"NTP_SERVERS" envSeparator:"," envDefault:"time.google.com,time.cloudflare.com,pool.ntp.org"`
	NTPSyncInterval int      `env:"NTP_SYNC_INTERVAL" envDefault:"300"`
	NTPQueryTimeout int      `env:"NTP_QUERY_TIMEOUT" envDefault:"5"`

	// TON Analytics
	TONAnalyticsEnabled       bool   `env:"TON_ANALYTICS_ENABLED" envDefault:"false"`
	TonAnalyticsURL           string `env:"TON_ANALYTICS_URL" envDefault:"https://analytics.ton.org/events"`
	TonAnalyticsBridgeVersion string `env:"TON_ANALYTICS_BRIDGE_VERSION" envDefault:"1.0.0"` // TODO start using build version
	TonAnalyticsBridgeURL     string `env:"TON_ANALYTICS_BRIDGE_URL" envDefault:"localhost"`
	TonAnalyticsNetworkId     string `env:"TON_ANALYTICS_NETWORK_ID" envDefault:"-239"`
}{}

// LoadConfig parses the process environment into Config. Logger configuration is no longer done here:
// the v3 binary configures slog via obs.Setup, and the v1 binary configures logrus via configureV1Logrus
// in cmd/bridge. Keeping this package logrus-free is what lets the v3 binary link without logrus.
func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		slog.Error("config parsing failed", "err", err)
		os.Exit(1)
	}
}
