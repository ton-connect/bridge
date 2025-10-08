package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime/debug"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal"
	"github.com/tonkeeper/bridge/internal/app"
	"github.com/tonkeeper/bridge/internal/config"
	bridge_middleware "github.com/tonkeeper/bridge/internal/middleware"
	"github.com/tonkeeper/bridge/internal/utils"
	handlerv1 "github.com/tonkeeper/bridge/internal/v1/handler"
	"github.com/tonkeeper/bridge/internal/v1/storage"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

func main() {
	log.Info(fmt.Sprintf("Bridge %s is running", internal.BridgeVersionRevision))
	config.LoadConfig()
	app.InitMetrics()

	// Global panic recovery function for goroutines
	recoverPanic := func(operation string) {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.WithFields(log.Fields{
				"operation": operation,
				"panic":     r,
				"stack":     string(stack),
			}).Error("Goroutine panic recovered")
		}
	}

	dbConn, err := storage.NewStorage(config.Config.PostgresURI)
	if err != nil {
		log.Fatalf("db connection %v", err)
	}

	healthManager := app.NewHealthManager()
	healthManager.UpdateHealthStatus(dbConn)
	go func() {
		defer recoverPanic("health-monitoring")
		healthManager.StartHealthMonitoring(dbConn)
	}()

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
		defer recoverPanic("metrics-server")
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Config.MetricsPort), mux))
	}()

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper: nil,
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			log.WithFields(log.Fields{
				"error":      err.Error(),
				"method":     c.Request().Method,
				"path":       c.Request().URL.Path,
				"remote_ip":  c.RealIP(),
				"user_agent": c.Request().UserAgent(),
				"stack":      string(stack),
			}).Error("Panic recovered")
			return err
		},
		DisableStackAll:   false,
		DisablePrintStack: true, // We handle logging ourselves
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

	h := handlerv1.NewHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second, extractor)

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
