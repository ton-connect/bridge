package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/tonkeeper/bridge/storage/memory"
	"github.com/tonkeeper/bridge/storage/pg"
	"golang.org/x/exp/slices"

	"github.com/tonkeeper/bridge/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(":9103", nil))
	}()

	e := echo.New()
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))
	e.Use(middleware.Logger())
	e.Use(connectionsLimitMiddleware(newAuthenticator()))

	h := newHandler(dbConn)

	registerHandlers(e, h)
	var existedPaths []string
	for _, r := range e.Routes() {
		existedPaths = append(existedPaths, r.Path)
	}
	p := prometheus.NewPrometheus("http", func(c echo.Context) bool {
		return !slices.Contains(existedPaths, c.Path())
	})
	e.Use(p.HandlerFunc)
	log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
}
