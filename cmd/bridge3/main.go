package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/app"
	"github.com/ton-connect/bridge/internal/config"
	bridge_middleware "github.com/ton-connect/bridge/internal/middleware"
	"github.com/ton-connect/bridge/internal/ntp"
	"github.com/ton-connect/bridge/internal/utils"
	handlerv3 "github.com/ton-connect/bridge/internal/v3/handler"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
	"github.com/ton-connect/bridge/tonmetrics"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

func main() {
	log.Info(fmt.Sprintf("Bridge3 %s is running", internal.BridgeVersionRevision))
	config.LoadConfig()
	app.InitMetrics()

	var timeProvider ntp.TimeProvider
	if config.Config.NTPEnabled {
		ntpClient := ntp.NewClient(ntp.Options{
			Servers:      config.Config.NTPServers,
			SyncInterval: time.Duration(config.Config.NTPSyncInterval) * time.Second,
			QueryTimeout: time.Duration(config.Config.NTPQueryTimeout) * time.Second,
		})
		ctx := context.Background()
		ntpClient.Start(ctx)
		defer ntpClient.Stop()
		timeProvider = ntpClient
		log.WithFields(log.Fields{
			"servers":       config.Config.NTPServers,
			"sync_interval": config.Config.NTPSyncInterval,
		}).Info("NTP synchronization enabled")
	} else {
		timeProvider = ntp.NewLocalTimeProvider()
		log.Info("NTP synchronization disabled, using local time")
	}
	tonAnalytics := tonmetrics.NewAnalyticsClient()

	dbURI := ""
	store := "memory"
	if config.Config.Storage != "" {
		store = config.Config.Storage
	}

	switch store {
	case "postgres":
		log.Info("Using PostgreSQL storage")
		dbURI = config.Config.PostgresURI
	case "valkey":
		log.Info("Using Valkey storage")
		dbURI = config.Config.ValkeyURI
	default:
		log.Info("Using in-memory storage as default")
		// No URI needed for memory storage
	}

	collector := analytics.NewCollector(200, tonAnalytics, 500*time.Millisecond)
	go collector.Run(context.Background())

	analyticsBuilder := analytics.NewEventBuilder(
		config.Config.TonAnalyticsBridgeURL,
		"bridge",
		"bridge",
		config.Config.TonAnalyticsBridgeVersion,
		config.Config.TonAnalyticsNetworkId,
	)

	dbConn, err := storagev3.NewStorage(store, dbURI, collector, analyticsBuilder)

	if err != nil {
		log.Fatalf("failed to create storage: %v", err)
	}
	if _, ok := dbConn.(*storagev3.MemStorage); ok {
		log.Info("Using in-memory storage")
		app.SetBridgeInfo("bridgev3", "memory")
	} else if _, ok := dbConn.(*storagev3.ValkeyStorage); ok {
		log.Info("Using Valkey/Redis storage")
		app.SetBridgeInfo("bridgev3", "valkey")
	} else {
		log.Info("Using PostgreSQL storage")
		app.SetBridgeInfo("bridgev3", "postgres")
	}
	healthManager := app.NewHealthManager()
	healthManager.UpdateHealthStatus(dbConn)
	go healthManager.StartHealthMonitoring(dbConn)

	extractor, err := utils.NewRealIPExtractor(config.Config.TrustedProxyRanges)
	if err != nil {
		log.Warnf("failed to create realIPExtractor: %v, using defaults", err)
		extractor, _ = utils.NewRealIPExtractor([]string{})
	}

	mux := http.NewServeMux()
	mux.Handle("/health", http.HandlerFunc(healthManager.HealthHandler))
	mux.Handle("/ready", http.HandlerFunc(healthManager.HealthHandler))
	mux.Handle("/version", http.HandlerFunc(app.VersionHandler))
	mux.Handle("/metrics", promhttp.Handler())
	if config.Config.PprofEnabled {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
	}
	go func() {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Config.MetricsPort), mux))
	}()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))
	e.Use(middleware.Logger())
	e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Skipper: func(c echo.Context) bool {
			if app.SkipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/message" {
				return true
			}
			return false
		},
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(config.Config.RPSLimit)),
	}))
	e.Use(app.ConnectionsLimitMiddleware(bridge_middleware.NewConnectionLimiter(config.Config.ConnectionsLimit, extractor), func(c echo.Context) bool {
		if app.SkipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/events" {
			return true
		}
		return false
	}))

	if config.Config.CorsEnable {
		corsConfig := middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     []string{"*"},
			AllowMethods:     []string{echo.GET, echo.POST, echo.OPTIONS},
			AllowHeaders:     []string{"DNT", "X-CustomHeader", "Keep-Alive", "User-Agent", "X-Requested-With", "If-Modified-Since", "Cache-Control", "Content-Type", "Authorization"},
			AllowCredentials: true,
			MaxAge:           86400,
		})
		e.Use(corsConfig)
	}

	h := handlerv3.NewHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second, extractor, timeProvider, collector, analyticsBuilder)

	e.GET("/bridge/events", h.EventRegistrationHandler)
	e.POST("/bridge/message", h.SendMessageHandler)
	e.POST("/bridge/verify", h.ConnectVerifyHandler)

	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	e.Use(p.HandlerFunc)
	if config.Config.SelfSignedTLS {
		cert, key, err := utils.GenerateSelfSignedCertificate()
		if err != nil {
			log.Fatalf("failed to generate self signed certificate: %v", err)
		}
		log.Fatal(e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key))
	} else {
		log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
	}
}
