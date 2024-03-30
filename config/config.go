package config

import (
	"log"

	"github.com/caarlos0/env/v6"
)

var Config = struct {
	Port                  int      `env:"PORT" envDefault:"8081"`
	DbURI                 string   `env:"POSTGRES_URI"`
	WebhookURL            string   `env:"WEBHOOK_URL"`
	CorsEnable            bool     `env:"CORS_ENABLE"`
	HeartbeatInterval     int      `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
	RPSLimit              int      `env:"RPS_LIMIT" envDefault:"1"`
	RateLimitsByPassToken []string `env:"RATE_LIMITS_BY_PASS_TOKEN"`
	ConnectionsLimit      int      `env:"CONNECTIONS_LIMIT" envDefault:"50"`
	SelfSignedTLS         bool     `env:"SELF_SIGNED_TLS" envDefault:"false"`
}{}

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}
}
