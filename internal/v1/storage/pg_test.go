package storage

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/ton-connect/bridge/internal/config"
)

func TestConfigurePoolSettings(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		setup    func()
		wantErr  bool
		validate func(*testing.T, *pgxpool.Config)
	}{
		{
			name: "default settings",
			uri:  "postgres://user:pass@localhost/test",
			setup: func() {
				config.Config.PostgresMaxConns = 4
				config.Config.PostgresMinConns = 0
				config.Config.PostgresMaxConnLifetime = "1h"
				config.Config.PostgresMaxConnLifetimeJitter = "10m"
				config.Config.PostgresMaxConnIdleTime = "30m"
				config.Config.PostgresHealthCheckPeriod = "1m"
				config.Config.PostgresLazyConnect = false
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *pgxpool.Config) {
				if cfg.MaxConns != 4 {
					t.Errorf("Expected MaxConns=4, got %d", cfg.MaxConns)
				}
				if cfg.MinConns != 0 {
					t.Errorf("Expected MinConns=0, got %d", cfg.MinConns)
				}
				if cfg.MaxConnLifetime != time.Hour {
					t.Errorf("Expected MaxConnLifetime=1h, got %v", cfg.MaxConnLifetime)
				}
			},
		},
		{
			name: "custom settings",
			uri:  "postgres://user:pass@localhost/test",
			setup: func() {
				config.Config.PostgresMaxConns = 10
				config.Config.PostgresMinConns = 2
				config.Config.PostgresMaxConnLifetime = "45m"
				config.Config.PostgresMaxConnLifetimeJitter = "5m"
				config.Config.PostgresMaxConnIdleTime = "15m"
				config.Config.PostgresHealthCheckPeriod = "30s"
				config.Config.PostgresLazyConnect = true
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *pgxpool.Config) {
				if cfg.MaxConns != 10 {
					t.Errorf("Expected MaxConns=10, got %d", cfg.MaxConns)
				}
				if cfg.MinConns != 2 {
					t.Errorf("Expected MinConns=2, got %d", cfg.MinConns)
				}
				if cfg.MaxConnLifetime != 45*time.Minute {
					t.Errorf("Expected MaxConnLifetime=45m, got %v", cfg.MaxConnLifetime)
				}
				if !cfg.LazyConnect {
					t.Error("Expected LazyConnect=true")
				}
			},
		},
		{
			name: "invalid duration uses pgxpool defaults",
			uri:  "postgres://user:pass@localhost/test",
			setup: func() {
				config.Config.PostgresMaxConns = 5
				config.Config.PostgresMinConns = 1
				config.Config.PostgresMaxConnLifetime = "invalid-duration"
				config.Config.PostgresMaxConnLifetimeJitter = "1m"
				config.Config.PostgresMaxConnIdleTime = "10m"
				config.Config.PostgresHealthCheckPeriod = "30s"
				config.Config.PostgresLazyConnect = false
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *pgxpool.Config) {
				if cfg.MaxConns != 5 {
					t.Errorf("Expected MaxConns=5, got %d", cfg.MaxConns)
				}
				// Invalid duration should keep pgxpool default, which is 0 initially
				// but pgxpool may set its own defaults internally
			},
		},
		{
			name:    "invalid URI",
			uri:     "not-a-valid-uri",
			setup:   func() {},
			wantErr: true,
			validate: func(t *testing.T, cfg *pgxpool.Config) {
				// Should not be called for error cases
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			poolConfig, err := configurePoolSettings(tt.uri)

			if (err != nil) != tt.wantErr {
				t.Errorf("configurePoolSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && poolConfig != nil {
				tt.validate(t, poolConfig)
			}
		})
	}
}
