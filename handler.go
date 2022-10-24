package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
)

type handler struct {
	Mux         sync.Mutex
	Connections map[string]*Session
}

func newHandler() *handler {
	return &handler{
		Mux:         sync.Mutex{},
		Connections: make(map[string]*Session),
	}
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := log.WithField("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return fmt.Errorf("streaming unsupported")
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)

	params := c.QueryParams()
	clientId, ok := params["client_id"]
	if !ok {
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	newSession := NewSession(clientId[0], &c) // sessionId = full clientId string

	clientIds := strings.Split(clientId[0], ",")
	// remove old connection
	for _, id := range clientIds {
		oldSes, ok := h.Connections[id]
		if ok {
			log.Infof("hijack old connection with id: %v", id)
			oldConnection, _, err := (*oldSes.Connection).Response().Hijack()
			if err != nil {
				errorMsg := fmt.Sprintf("old connection  hijack error: %v", err)
				log.Errorf(errorMsg)
				return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
			}
			err = oldConnection.Close()
			if err != nil {
				errorMsg := fmt.Sprintf("old connection  close error: %v", err)
				log.Errorf(errorMsg)
				return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
			}
		}

		h.Mux.Lock()
		h.Connections[id] = newSession
		h.Mux.Unlock()
	}

	notify := c.Request().Context().Done()
	go func() {
		<-notify
		newSession.SessionCloser <- true
		for _, id := range clientIds {
			h.Mux.Lock()
			delete(h.Connections, id)
			h.Mux.Unlock()
		}
		log.Infof("remove connection wit clientId: %v from map", clientId[0])
	}()

	for {
		msg, open := <-newSession.MessageCh
		if !open {
			break
		}
		c.JSON(http.StatusOK, msg)
		c.Response().Flush()
	}
	return nil
}

func (h *handler) SendMessageHandler(c echo.Context) error {
	ctx := c.Request().Context()
	log := log.WithContext(ctx).WithField("prefix", "SendMessageHandler")
	params := c.QueryParams()
	clientId, ok := params["client_id"]
	if !ok {
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	if _, ok := h.Connections[clientId[0]]; !ok {
		errorMsg := fmt.Sprintf("client with client_id: %v not connected", clientId[0])
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	toId, ok := params["to"]
	if !ok {
		errorMsg := "param \"to\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	toIdSession, ok := h.Connections[toId[0]]
	if !ok {
		errorMsg := fmt.Sprintf("client with client_id: %v not connected", toId[0])
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		errorMsg := "param \"ttl\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}
	if ttl > 300 { // TODO: config
		errorMsg := "param \"ttl\" too high"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}

	done := make(chan interface{})

	toIdSession.addMessageToDeque(clientId[0], ttl, message, done)

	ttlTimer := time.NewTimer(time.Duration(ttl) * time.Second)

	notify := c.Request().Context().Done()
	go func() {
		<-notify
		ttlTimer.Stop()
		return
	}()

	for {
		select {
		case <-done:
			ttlTimer.Stop()
			return c.JSON(http.StatusOK, HttpResOk())
		case <-ttlTimer.C:
			log.Info("message expired")
			return c.JSON(HttpResError("timeout", http.StatusBadRequest))
		}
	}
}
