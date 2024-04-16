package config

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
)

var Config = struct {
	Port                  int      `env:"PORT" envDefault:"8081"`
	DbURI                 string   `env:"POSTGRES_URI"`
	WebhookURL            string   `env:"WEBHOOK_URL"`
	CorsEnable            bool     `env:"CORS_ENABLE"`
	HeartbeatInterval     int      `env:"HEARTBEAT_INTERVAL" envDefault:"10"`
	RPSLimit              int      `env:"RPS_LIMIT" envDefault:"1"`
	RateLimitsByPassToken []string `env:"RATE_LIMITS_BY_PASS_TOKEN"` // TODO: remove this env, read from file
	ConnectionsLimit      int      `env:"CONNECTIONS_LIMIT" envDefault:"50"`
	SelfSignedTLS         bool     `env:"SELF_SIGNED_TLS" envDefault:"false"`
}{}

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}

	go refreshUnlimitedTokens()
}

func refreshUnlimitedTokens() {
	type UnlimitedTokens struct {
		Tokens []string `json:"tokens"`
	}
	refresh := func() []string {
		file, err := os.ReadFile("config/unlimited_tokens.json")
		if err != nil {
			log.Printf("failed to read unlimited tokens file: %v", err)
			return nil
		}
		var tokens UnlimitedTokens
		if err = json.Unmarshal(file, &tokens); err != nil {
			log.Printf("failed to convert unlimited tokens: %v", err)
			return nil
		}
		return tokens.Tokens
	}
	for {
		tokens := refresh()
		if tokens != nil {
			Config.RateLimitsByPassToken = tokens
		}
		time.Sleep(time.Minute * 5)
	}
}
