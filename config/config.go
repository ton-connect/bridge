package config

import (
	"log"

	"github.com/caarlos0/env/v6"
)

var Config = struct {
	Port  int    `env:"PORT" envDefault:"8081"`
	DbURI string `env:"POSTGRES_URI"`
}{}

func LoadConfig() {
	if err := env.Parse(&Config); err != nil {
		log.Fatalf("config parsing failed: %v\n", err)
	}
}
