package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/ton-connect/bridge/internal"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/app"
	"github.com/ton-connect/bridge/internal/config"
	bridge_middleware "github.com/ton-connect/bridge/internal/middleware"
	"github.com/ton-connect/bridge/internal/ntp"
	"github.com/ton-connect/bridge/internal/obs"
	"github.com/ton-connect/bridge/internal/utils"
	handlerv3 "github.com/ton-connect/bridge/internal/v3/handler"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
	"github.com/ton-connect/bridge/tonmetrics"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

func main() {
	config.LoadConfig()
	slog.SetDefault(obs.Setup(os.Stdout, config.Config.LogLevel, "bridge"))
	slog.Info("Bridge3 is running", "revision", internal.BridgeVersionRevision)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
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
		slog.Info("NTP synchronization enabled", "servers", config.Config.NTPServers, "sync_interval", config.Config.NTPSyncInterval)
	} else {
		timeProvider = ntp.NewLocalTimeProvider()
		slog.Info("NTP synchronization disabled, using local time")
	}
	tonAnalytics := tonmetrics.NewAnalyticsClient()

	dbURI := ""
	store := "memory"
	if config.Config.Storage != "" {
		store = config.Config.Storage
	}

	switch store {
	case "postgres":
		slog.Info("Using PostgreSQL storage")
		dbURI = config.Config.PostgresURI
	case "valkey":
		slog.Info("Using Valkey storage")
		dbURI = config.Config.ValkeyURI
	default:
		slog.Info("Using in-memory storage as default")
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
		slog.Error("failed to create storage", "err", err)
		os.Exit(1)
	}
	if _, ok := dbConn.(*storagev3.MemStorage); ok {
		slog.Info("Using in-memory storage")
		app.SetBridgeInfo("bridgev3", "memory")
	} else if _, ok := dbConn.(*storagev3.ValkeyStorage); ok {
		slog.Info("Using Valkey/Redis storage")
		app.SetBridgeInfo("bridgev3", "valkey")
	} else {
		slog.Info("Using PostgreSQL storage")
		app.SetBridgeInfo("bridgev3", "postgres")
	}
	healthManager := app.NewHealthManager()
	healthManager.UpdateHealthStatus(dbConn)
	go healthManager.StartHealthMonitoring(dbConn)

	extractor, err := utils.NewRealIPExtractor(config.Config.TrustedProxyRanges)
	if err != nil {
		slog.Warn("failed to create realIPExtractor, using defaults", "err", err)
		extractor, _ = utils.NewRealIPExtractor([]string{})
	}

	mux := http.NewServeMux()
	mux.Handle("/health", http.HandlerFunc(healthManager.HealthHandler))
	mux.Handle("/ready", http.HandlerFunc(healthManager.HealthHandler))
	mux.Handle("/healthz", http.HandlerFunc(healthManager.LivenessHandler))
	mux.Handle("/readyz", http.HandlerFunc(healthManager.ReadinessHandler))
	mux.Handle("/version", http.HandlerFunc(app.VersionHandler))
	mux.Handle("/metrics", promhttp.Handler())
	if config.Config.PprofEnabled {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
	}
	go func() {
		slog.Error("metrics server failed", "err", http.ListenAndServe(fmt.Sprintf(":%d", config.Config.MetricsPort), mux))
		os.Exit(1)
	}()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))
	// Structured access log via the modern RequestLogger API, emitted through the default slog logger
	// (JSON, configured in obs.Setup) so every request line carries a `msg` and a status-derived `level`,
	// consistent with the app logs. Replaces the deprecated middleware.Logger(), whose hardcoded JSON
	// template had neither field.
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogHost:      true,
		LogUserAgent: true,
		LogError:     true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			switch {
			case v.Status >= 500:
				level = slog.LevelError
			case v.Status >= 400:
				level = slog.LevelWarn
			}
			attrs := []slog.Attr{
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.String("latency", v.Latency.String()),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("host", v.Host),
				slog.String("user_agent", v.UserAgent),
			}
			if v.Error != nil {
				attrs = append(attrs, slog.String("error", v.Error.Error()))
			}
			slog.LogAttrs(c.Request().Context(), level, "http_request", attrs...)
			return nil
		},
	}))
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
	go func() {
		var err error
		if config.Config.SelfSignedTLS {
			cert, key, certErr := utils.GenerateSelfSignedCertificate()
			if certErr != nil {
				slog.Error("failed to generate self signed certificate", "err", certErr)
				os.Exit(1)
			}
			err = e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key)
		} else {
			err = e.Start(fmt.Sprintf(":%v", config.Config.Port))
		}
		// Shutdown closes the listener and returns ErrServerClosed, the normal stop path.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("bridge server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	// Graceful shutdown: flip /readyz to 503 so the load balancer takes the pod out of rotation
	// while it is still listening, wait the drain window, then drain in-flight requests. SSE streams
	// never complete on their own, so the bounded ShutdownTimeout ends the wait (clients reconnect).
	slog.Info("SIGTERM received, draining")
	healthManager.SetDraining()
	time.Sleep(time.Duration(config.Config.ShutdownDrainDelay) * time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Config.ShutdownTimeout)*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Error("bridge shutdown", "err", err)
	}
	slog.Info("bridge stopped")
}
