package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/storage"
)

var (
	activeConnectionMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "number_of_acitve_connections",
		Help: "The number of active connections",
	})
	activeSubscriptionsMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "number_of_active_subscriptions",
		Help: "The number of active subscriptions",
	})
	transferedMessagesNumMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_transfered_messages",
		Help: "The total number of transfered_messages",
	})
	deliveredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_delivered_messages",
		Help: "The total number of delivered_messages",
	})
	expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_expired_messages",
		Help: "The total number of expired messages",
	})
	badRequestMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_bad_requests",
		Help: "The total number of bad requests",
	})
)

type handler struct {
	Mux         sync.Mutex
	Connections map[string]*Session
	storage     *storage.Storage[storage.MessageWithTtl]
}

func newHandler() *handler {

	h := handler{
		Mux:         sync.Mutex{},
		Connections: make(map[string]*Session),
		storage:     storage.NewStorage[storage.MessageWithTtl](),
	}

	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := log.WithField("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(HttpResError("streaming unsupported", http.StatusBadRequest))
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().WriteHeader(http.StatusOK)

	params := c.QueryParams()
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	clientIds := strings.Split(clientId[0], ",")

	newSession := NewSession(h.storage, clientIds)
	for _, id := range clientIds {
		h.Mux.Lock()
		con, ok := h.Connections[id]
		if ok {
			con.mux.Lock()
			for i := range con.ClientIds {
				if con.ClientIds[i] == id {
					con.ClientIds[i] = con.ClientIds[len(con.ClientIds)-1]
					con.ClientIds = con.ClientIds[:len(con.ClientIds)-1]
				}
			}
			con.mux.Unlock()
		}
		h.Connections[id] = newSession
		h.Mux.Unlock()
	}
	notify := c.Request().Context().Done()
	go func() {
		<-notify
		h.Mux.Lock()
		defer h.Mux.Unlock()
		newSession.mux.Lock()
		for _, id := range newSession.ClientIds {
			delete(h.Connections, id)
		}
		newSession.Closer <- true
		newSession.mux.Unlock()
		log.Infof("connection: %v closed", clientId[0])
	}()

	for {
		msg, open := <-newSession.MessageCh
		if !open {
			break
		}
		c.JSON(http.StatusOK, msg)
		c.Response().Flush()
	}
	log.Info("connection closed")
	return nil
}

func (h *handler) SendMessageHandler(c echo.Context) error {
	ctx := c.Request().Context()
	log := log.WithContext(ctx).WithField("prefix", "SendMessageHandler")

	params := c.QueryParams()
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	toId, ok := params["to"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"to\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}
	if ttl > 300 { // TODO: config
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" too high"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}

	done := make(chan interface{}, 1)
	remove := make(chan interface{}, 1)
	h.storage.Add(toId[0], storage.MessageWithTtl{
		PushTime:      time.Now(),
		Ttl:           ttl,
		From:          clientId[0],
		To:            toId[0],
		Message:       message,
		RequestCloser: done,
		RemoveMessage: remove,
	})
	transferedMessagesNumMetric.Inc()
	ttlTimer := time.NewTimer(time.Duration(ttl) * time.Second)

	for {
		select {
		case <-c.Request().Context().Done():
			ttlTimer.Stop()
			log.Info("connection has been closed by client. Remove message from queue")
			remove <- true
			return nil
		case <-done:
			deliveredMessagesMetric.Inc()
			ttlTimer.Stop()
			return c.JSON(http.StatusOK, HttpResOk())
		case <-ttlTimer.C:
			log.Info("message expired")
			expiredMessagesMetric.Inc()
			return c.JSON(HttpResError("timeout", http.StatusBadRequest))
		}
	}
}
