package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/config"
	"github.com/tonkeeper/bridge/storage/memory"
	"github.com/tonkeeper/bridge/storage/pg"
	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"
)

func main() {
	log.Info("Bridge is running")
	config.LoadConfig()
	var (
		dbConn db
		err    error
	)
	if config.Config.DbURI != "" {
		dbConn, err = pg.NewStorage(config.Config.DbURI)
		if err != nil {
			log.Fatalf("db connection %v", err)
		}
	} else {
		dbConn = memory.NewStorage()
	}

	extractor, err := newRealIPExtractor(config.Config.TrustedProxyRanges)
	if err != nil {
		log.Warnf("failed to create realIPExtractor: %v, using defaults", err)
		extractor, _ = newRealIPExtractor([]string{})
	}

	mux := http.NewServeMux()
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
	e.Use(connectionsLimitMiddleware(newConnectionLimiter(config.Config.ConnectionsLimit, extractor), func(c echo.Context) bool {
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

	h := newHandler(dbConn, time.Duration(config.Config.HeartbeatInterval)*time.Second, extractor)

	registerHandlers(e, h)
	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	e.Use(p.HandlerFunc)
	if config.Config.SelfSignedTLS {
		cert, key, err := generateSelfSignedCertificate()
		if err != nil {
			log.Fatalf("failed to generate self signed certificate: %v", err)
		}
		log.Fatal(e.StartTLS(fmt.Sprintf(":%v", config.Config.Port), cert, key))
	} else {
		log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
	}
}
