package main

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

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
	badRequestMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_bad_requests",
		Help: "The total number of bad requests",
	})
)

type handler struct {
	Mux         sync.Mutex
	Connections map[string]*Session
	storage     *storage.Storage
}

func newHandler() *handler {

	h := handler{
		Mux:         sync.Mutex{},
		Connections: make(map[string]*Session),
		storage:     storage.NewStorage(),
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
	log.Infof("make new session with ids: %v", clientIds)
	newSession := NewSession(h.storage, clientIds)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.Mux.Lock()
		con, ok := h.Connections[id]
		activeSubscriptionsMetric.Inc()
		if ok {
			log.Infof("remove id: %v from old conn", id)
			con.mux.Lock()
			for i := range con.ClientIds {
				if con.ClientIds[i] == id {
					con.ClientIds[i] = con.ClientIds[len(con.ClientIds)-1]
					con.ClientIds = con.ClientIds[:len(con.ClientIds)-1]
					activeSubscriptionsMetric.Dec()
					break
				}
			}
			if len(con.ClientIds) == 0 {
				con.Closer <- true
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
			activeSubscriptionsMetric.Dec()
		}
		newSession.Closer <- true
		newSession.mux.Unlock()
		activeConnectionMetric.Dec()
		log.Infof("connection: %v closed", newSession.ClientIds)
	}()

	for {
		msg, open := <-newSession.MessageCh
		if !open {
			break
		}
		c.JSON(http.StatusOK, msg)
		c.Response().Flush()
		deliveredMessagesMetric.Inc()
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
	mes := BridgeMessage{
		From:    clientId[0],
		Message: message,
	}
	transferedMessagesNumMetric.Inc()
	ses, ok := h.Connections[toId[0]]
	if ok {
		ses.AddMessageToQueue(mes)
	} else {
		h.storage.Add(toId[0], ttl, mes)
	}

	return c.JSON(http.StatusOK, HttpResOk())

}
