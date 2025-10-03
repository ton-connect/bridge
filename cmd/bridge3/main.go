package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	client_prometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal"
	"github.com/tonkeeper/bridge/internal/config"
	bridge_middleware "github.com/tonkeeper/bridge/internal/middleware"
	"github.com/tonkeeper/bridge/internal/utils"
	handlerv3 "github.com/tonkeeper/bridge/internal/v3/handler"
	storagev3 "github.com/tonkeeper/bridge/internal/v3/storage"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

var (
	tokenUsageMetric = promauto.NewCounterVec(client_prometheus.CounterOpts{
		Name: "bridge_token_usage",
	}, []string{"token"})

	healthMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_health_status",
		Help: "Health status of the bridge (1 = healthy, 0 = unhealthy)",
	})
	readyMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_ready_status",
		Help: "Ready status of the bridge (1 = ready, 0 = not ready)",
	})
	versionMetric = client_prometheus.NewGaugeVec(client_prometheus.GaugeOpts{
		Name: "bridge_version",
		Help: "Bridge version",
	}, []string{"version"})

	// Shared health status updated by background goroutine
	healthy = 0
)

func init() {
	client_prometheus.MustRegister(healthMetric)
	client_prometheus.MustRegister(readyMetric)
	client_prometheus.MustRegister(versionMetric)
	versionMetric.WithLabelValues(internal.BridgeVersionRevision).Set(1)
}

func skipRateLimitsByToken(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return false
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	exist := slices.Contains(config.Config.RateLimitsByPassToken, token)
	if exist {
		tokenUsageMetric.WithLabelValues(token).Inc()
		return true
	}
	return false
}

func connectionsLimitMiddleware(counter *bridge_middleware.ConnectionsLimiter, skipper func(c echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper(c) {
				return next(c)
			}
			release, err := counter.LeaseConnection(c.Request())
			if err != nil {
				return c.JSON(utils.HttpResError(err.Error(), http.StatusTooManyRequests))
			}
			defer release()
			return next(c)
		}
	}
}

func updateHealthStatus(dbConn storagev3.Storage) {
	if err := dbConn.HealthCheck(); err != nil {
		healthy = 0
	} else {
		healthy = 1
	}

	healthMetric.Set(float64(healthy))
	readyMetric.Set(float64(healthy))
}

func main() {
	log.Info(fmt.Sprintf("Bridge3 %s is running", internal.BridgeVersionRevision))
	config.LoadConfig()

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

	dbConn, err := storagev3.NewStorage(store, dbURI)

	if err != nil {
		log.Fatalf("failed to create storage: %v", err)
	}
	if _, ok := dbConn.(*storagev3.MemStorage); ok {
		log.Info("Using in-memory storage")
	} else if _, ok := dbConn.(*storagev3.ValkeyStorage); ok {
		log.Info("Using Valkey/Redis storage")
	} else {
		log.Info("Using PostgreSQL storage")
	}
	updateHealthStatus(dbConn)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			updateHealthStatus(dbConn)
		}
	}()

	extractor, err := utils.NewRealIPExtractor(config.Config.TrustedProxyRanges)
	if err != nil {
		log.Warnf("failed to create realIPExtractor: %v, using defaults", err)
		extractor, _ = utils.NewRealIPExtractor([]string{})
	}

	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if healthy == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, err := fmt.Fprintf(w, `{"status":"unhealthy"}`+"\n")
			if err != nil {
				log.Errorf("health response write error: %v", err)
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, `{"status":"ok"}`+"\n")
		if err != nil {
			log.Errorf("health response write error: %v", err)
		}
	}

	versionHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`{"version":"%s"}`, internal.BridgeVersionRevision)
		_, err := fmt.Fprintf(w, "%s", response+"\n")
		if err != nil {
			log.Errorf("version response write error: %v", err)
		}
	}

	mux.Handle("/health", http.HandlerFunc(healthHandler))
	mux.Handle("/ready", http.HandlerFunc(healthHandler))
	mux.Handle("/version", http.HandlerFunc(versionHandler))
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
			if skipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/message" {
				return true
			}
			return false
		},
		Store: middleware.NewRateLimiterMemoryStore(rate.Limit(config.Config.RPSLimit)),
	}))
	e.Use(connectionsLimitMiddleware(bridge_middleware.NewConnectionLimiter(config.Config.ConnectionsLimit, extractor), func(c echo.Context) bool {
		if skipRateLimitsByToken(c.Request()) || c.Path() != "/bridge/events" {
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

	h := handlerv3.NewHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second, extractor)

	e.GET("/bridge/events", h.EventRegistrationHandler)
	e.POST("/bridge/message", h.SendMessageHandler)
	// e.POST("/bridge/verify", h.ConnectVerifyHandler)

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
