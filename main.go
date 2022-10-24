package main

import (
	"bridge/config"
	"fmt"
	"net/http"

	_ "net/http/pprof"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.Info("Bridge is running")
	config.LoadConfig()
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
	p := prometheus.NewPrometheus("http", nil)
	p.Use(e)
	h := newHandler()

	registerHandlers(e, h)

	log.Fatal(e.Start(fmt.Sprintf(":%v", config.Config.Port)))
}
