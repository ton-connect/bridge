package config

import (
	"log"

	"github.com/caarlos0/env/v6"
)

var Config = struct {
	Port int `env:"PORT" envDefault:"8081"`
}{}

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}
}
