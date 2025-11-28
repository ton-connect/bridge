package config

import (
	"log"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/sirupsen/logrus"
)

var Config = struct {
	// Core Settings
	LogLevel     string `env:"LOG_LEVEL" envDefault:"info"`
	Port         int    `env:"PORT" envDefault:"8081"`
	MetricsPort  int    `env:"METRICS_PORT" envDefault:"9103"`
	PprofEnabled bool   `env:"PPROF_ENABLED" envDefault:"true"`

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
	MaxBodySize           int64    `env:"MAX_BODY_SIZE" envDefault:"10485760"` // 10 MB
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

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}

	level, err := logrus.ParseLevel(strings.ToLower(Config.LogLevel))
	if err != nil {
		log.Printf("Invalid LOG_LEVEL '%s', using default 'info'. Valid levels: panic, fatal, error, warn, info, debug, trace", Config.LogLevel)
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)
}
