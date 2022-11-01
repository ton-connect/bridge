package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func registerHandlers(e *echo.Echo, h *handler) {
	bridge := e.Group("/bridge")
	bridge.GET("/events", h.EventRegistrationHandler, middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET"},
	}))
	bridge.POST("/message", h.SendMessageHandler, middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"POST"},
	}), middleware.CORSWithConfig(cors))
}
