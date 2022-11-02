package main

import (
	"encoding/json"
	"fmt"
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
	"github.com/tonkeeper/bridge/datatype"
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
	Mux         sync.RWMutex
	Connections map[string][]*Session
	storage     *storage.Storage
}

func newHandler(db *storage.Storage) *handler {

	h := handler{
		Mux:         sync.RWMutex{},
		Connections: make(map[string][]*Session),
		storage:     db,
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
	fmt.Fprint(c.Response(), "\n")
	c.Response().Flush()
	params := c.QueryParams()

	var lastEventID int64
	var err error
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventID, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			c.JSON(HttpResError("Last-Event-ID should be int", http.StatusBadRequest))

		}
	}
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	clientIds := strings.Split(clientId[0], ",")
	log.Infof("make new session with ids: %v", clientIds)

	newSession := NewSession(clientId[0], h.storage, clientIds, lastEventID)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.Mux.Lock()
		h.Connections[id] = append(h.Connections[id], newSession)
		h.Mux.Unlock()
		activeSubscriptionsMetric.Inc()
	}
	notify := c.Request().Context().Done()
	go func() {
		<-notify
		close(newSession.Closer)
		for _, id := range clientIds {
			h.Mux.Lock()
			sessions, ok := h.Connections[id]
			h.Mux.Unlock()
			if ok {
				for i := range sessions {
					if sessions[i].SessionId == clientId[0] {
						sessions[i] = sessions[len(sessions)-1]
						sessions = sessions[:len(sessions)-1]
						h.Mux.Lock()
						h.Connections[id] = sessions
						if len(sessions) == 0 {
							delete(h.Connections, id)
						}
						h.Mux.Unlock()
						break
					}
				}
			}
			activeSubscriptionsMetric.Dec()
		}
		log.Infof("connection: %v closed", newSession.ClientIds)
	}()

	newSession.Start()

	for msg := range newSession.MessageCh {
		fmt.Fprintf(c.Response(), "id: %v\ndata: %v\n\n", msg.EventId, string(msg.Message)) //
		c.Response().Flush()
		deliveredMessagesMetric.Inc()
	}
	activeConnectionMetric.Dec()
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
	mes, err := json.Marshal(datatype.BridgeMessage{
		From:    clientId[0],
		Message: string(message),
	})
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}
	sessions, ok := h.Connections[toId[0]]
	if ok {
		for _, ses := range sessions {
			ses.AddMessageToQueue(ctx, datatype.SseMessage{
				EventId: time.Now().UnixMicro(),
				Message: mes,
			})
		}
	}
	err = h.storage.Add(ctx, toId[0], time.Now().UnixMicro(), ttl, mes)
	if err != nil {
		log.Errorf("db error: %v", err)
	}
	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, HttpResOk())

}
