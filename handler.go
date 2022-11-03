package main

import (
	"context"
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

type streams struct {
	Connections map[string][]*Session
}
type handler struct {
	Mux     sync.RWMutex
	Streams streams
	storage *storage.Storage
	Remover chan *Session
}

func newHandler(db *storage.Storage) *handler {
	h := handler{
		Mux: sync.RWMutex{},
		Streams: streams{
			Connections: make(map[string][]*Session),
		},
		storage: db,
		Remover: make(chan *Session, 10),
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

	var lastEventId int64
	var err error
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
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
	session := h.CreateSession(clientId[0], clientIds, lastEventId)

	notify := c.Request().Context().Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session)
		log.Infof("connection: %v closed", session.ClientIds)
	}()

	session.Start()
	for msg := range session.MessageCh {
		fmt.Fprintf(c.Response(), "id: %v\ndata: %v\n\n", msg.EventId, string(msg.Message))
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
	sseMessage := datatype.SseMessage{
		EventId: time.Now().UnixMicro(),
		Message: mes,
	}
	if ok {
		for _, ses := range h.Streams.Connections[toId[0]] {
			ses.AddMessageToQueue(ctx, sseMessage)
		}
	}
	go func() {
		log := log.WithField("prefix", "SendMessageHandler.storge.Add")
		err = h.storage.Add(context.Background(), toId[0], ttl, sseMessage)
		if err != nil {
			log.Errorf("db error: %v", err)
		}
	}()

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, HttpResOk())

}

func (h *handler) removeConnection(s *Session) {
	log := log.WithField("prefix", "removeConnection")
	log.Infof("remove session: %v", s.ClientIds)
	for _, id := range s.ClientIds {
		h.Mux.RLock()
		ses := h.Streams.Connections[id]
		h.Mux.RUnlock()
		for i := range ses {
			if ses[i] == s {
				ses[i] = ses[len(ses)-1]
				ses = ses[:len(ses)-1]
				break
			}
		}
		if len(ses) == 0 {
			h.Mux.Lock()
			delete(h.Streams.Connections, id)
			h.Mux.Unlock()
		}
	}
	activeSubscriptionsMetric.Dec()

}

func (h *handler) CreateSession(sessionId string, clientIds []string, lastEventId int64) *Session {
	log := log.WithField("prefix", "CreateSession")
	log.Infof("make new session with ids: %v", clientIds)
	session := NewSession(h.storage, clientIds, lastEventId)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.Mux.Lock()
		h.Streams.Connections[id] = append(h.Streams.Connections[id], session)
		h.Mux.Unlock()
		activeSubscriptionsMetric.Inc()
	}
	return session
}
