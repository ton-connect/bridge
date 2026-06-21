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
	"strings"
	"syscall"
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
	"github.com/ton-connect/bridge/internal/obs"
	"github.com/ton-connect/bridge/internal/utils"
	handlerv1 "github.com/ton-connect/bridge/internal/v1/handler"
	"github.com/ton-connect/bridge/internal/v1/storage"
	"github.com/ton-connect/bridge/tonmetrics"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

// configureV1Logrus configures the global logrus logger that the still-on-logrus v1 code uses: JSON
// output at the configured level (an unknown level falls back to info). The v3 path logs through slog
// (obs.Setup); this keeps v1's logrus output structured and consistent until the v1 code is migrated
// off logrus. It lives here, in the v1 binary, so the shared config package stays logrus-free and the
// v3 binary links without logrus.
func configureV1Logrus() {
	level, err := log.ParseLevel(strings.ToLower(config.Config.LogLevel))
	if err != nil {
		slog.Warn("invalid LOG_LEVEL, using default", "value", config.Config.LogLevel, "default", "info")
		level = log.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	// Install a JSON slog default before config loads so a config-parse failure logs on-contract
	// (JSON + service/git_sha); reconfigure to the configured level once config is loaded.
	slog.SetDefault(obs.Setup(os.Stdout, "info", "bridge"))
	log.Info(fmt.Sprintf("Bridge %s is running", internal.BridgeVersionRevision))
	config.LoadConfig()
	slog.SetDefault(obs.Setup(os.Stdout, config.Config.LogLevel, "bridge"))
	configureV1Logrus()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	app.InitMetrics()
	if config.Config.PostgresURI == "" {
		app.SetBridgeInfo("bridgev1", "memory")
	} else {
		app.SetBridgeInfo("bridgev1", "postgres")
	}

	tonAnalytics := tonmetrics.NewAnalyticsClient()

	collector := analytics.NewCollector(200, tonAnalytics, 500*time.Millisecond)
	go collector.Run(context.Background())

	analyticsBuilder := analytics.NewEventBuilder(
		config.Config.TonAnalyticsBridgeURL,
		"bridge",
		"bridge",
		config.Config.TonAnalyticsBridgeVersion,
		config.Config.TonAnalyticsNetworkId,
	)

	dbConn, err := storage.NewStorage(config.Config.PostgresURI, collector, analyticsBuilder)
	if err != nil {
		log.Fatalf("db connection %v", err)
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
	mux.Handle("/healthz", http.HandlerFunc(healthManager.LivenessHandler))
	mux.Handle("/readyz", http.HandlerFunc(healthManager.ReadinessHandler))
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
	// Structured access log via the modern RequestLogger API, emitted through logrus (JSON formatter
	// set in config.LoadConfig) so every request line carries a `msg` and a status-derived `level`,
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
			entry := log.WithFields(log.Fields{
				"method":     v.Method,
				"uri":        v.URI,
				"status":     v.Status,
				"latency":    v.Latency.String(),
				"remote_ip":  v.RemoteIP,
				"host":       v.Host,
				"user_agent": v.UserAgent,
			})
			if v.Error != nil {
				entry = entry.WithField("error", v.Error.Error())
			}
			switch {
			case v.Status >= 500:
				entry.Error("http_request")
			case v.Status >= 400:
				entry.Warn("http_request")
			default:
				entry.Info("http_request")
			}
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

	h := handlerv1.NewHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second, extractor, collector, analyticsBuilder)

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
				log.Fatalf("failed to generate self signed certificate: %v", certErr)
			}
			err = e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key)
		} else {
			err = e.Start(fmt.Sprintf(":%v", config.Config.Port))
		}
		// Shutdown closes the listener and returns ErrServerClosed, the normal stop path.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("bridge server: %v", err)
		}
	}()

	<-ctx.Done()
	// Graceful shutdown: flip /readyz to 503 so the load balancer takes the pod out of rotation
	// while it is still listening, wait the drain window, then drain in-flight requests. SSE streams
	// never complete on their own, so the bounded ShutdownTimeout ends the wait (clients reconnect).
	log.Info("SIGTERM received, draining")
	healthManager.SetDraining()
	time.Sleep(time.Duration(config.Config.ShutdownDrainDelay) * time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Config.ShutdownTimeout)*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Errorf("bridge shutdown: %v", err)
	}
	log.Info("bridge stopped")
}
