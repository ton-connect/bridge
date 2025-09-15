package config

import (
	"log"
	"strings"

	"github.com/caarlos0/env/v6"
	"github.com/sirupsen/logrus"
)

var Config = struct {
	LogLevel               string   `env:"LOG_LEVEL" envDefault:"info"`
	Port                   int      `env:"PORT" envDefault:"8081"`
	DbURI                  string   `env:"POSTGRES_URI"`
	WebhookURL             string   `env:"WEBHOOK_URL"`
	CopyToURL              string   `env:"COPY_TO_URL"`
	CorsEnable             bool     `env:"CORS_ENABLE"`
	HeartbeatInterval      int      `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
	RPSLimit               int      `env:"RPS_LIMIT" envDefault:"1"`
	RateLimitsByPassToken  []string `env:"RATE_LIMITS_BY_PASS_TOKEN"`
	ConnectionsLimit       int      `env:"CONNECTIONS_LIMIT" envDefault:"50"`
	SelfSignedTLS          bool     `env:"SELF_SIGNED_TLS" envDefault:"false"`
	ConnectCacheSize       int      `env:"CONNECT_CACHE_SIZE" envDefault:"2000000"`
	ConnectCacheTTL        int      `env:"CONNECT_CACHE_TTL" envDefault:"300"`
	DisconnectEventsTTL    int64    `env:"DISCONNECT_EVENTS_TTL" envDefault:"3600"`
	DisconnectEventMaxSize int      `env:"DISCONNECT_EVENT_MAX_SIZE" envDefault:"512"`
	TrustedProxyRanges     []string `env:"TRUSTED_PROXY_RANGES" envDefault:"0.0.0.0/0"`
	MaxBodySize            int64    `env:"MAX_BODY_SIZE" envDefault:"10485760"` // 10 MB
	PprofEnabled           bool     `env:"PPROF_ENABLED" envDefault:"true"`
	TFAnalyticsEnabled     bool     `env:"TF_ANALYTICS_ENABLED" envDefault:"false"`
	BridgeName             string   `env:"BRIDGE_NAME" envDefault:"ton-connect-bridge"`
	BridgeVersion          string   `env:"BRIDGE_VERSION" envDefault:"1.0.0"` // TODO start using build version
	BridgeURL              string   `env:"BRIDGE_URL" envDefault:"localhost"`
	Environment            string   `env:"ENVIRONMENT" envDefault:"production"`
	NetworkId              string   `env:"NETWORK_ID" envDefault:"-239"`
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
