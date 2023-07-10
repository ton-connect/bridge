package main

import (
	"fmt"
	"github.com/labstack/echo-contrib/prometheus"
	"github.com/tonkeeper/bridge/storage/memory"
	"github.com/tonkeeper/bridge/storage/pg"
	"golang.org/x/exp/slices"
	"net/http"

	"github.com/tonkeeper/bridge/config"

	_ "net/http/pprof"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.Info("Bridge is running")
	config.LoadConfig()
	var db db
	var err error
	if config.Config.DbURI != "" {
		db, err = pg.NewStorage(config.Config.DbURI)
		if err != nil {
			log.Fatalf("db connection %v", err)
		}
	} else {
		db = memory.NewStorage()
	}

	metricsMux := http.NewServeMux()

	metricsMux.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(":9103", metricsMux))
	}()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))
	e.Use(middleware.Logger())

	h := newHandler(db)

	registerHandlers(e, h)
	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	p.Use(e)
	log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
}
