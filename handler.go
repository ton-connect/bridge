package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/config"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/storage"
)

var validHeartbeatTypes = map[string]string{
	"legacy":  "event: heartbeat\n\n",
	"message": "event: message\r\ndata: heartbeat\r\n\r\n",
}

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
	clientIdsPerConnectionMetric = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "number_of_client_ids_per_connection",
		Buckets: []float64{1, 2, 3, 4, 5, 10, 20, 30, 40, 50, 100},
	})
)

type stream struct {
	Sessions []*Session
	mux      sync.RWMutex
}
type handler struct {
	Mux               sync.RWMutex
	Connections       map[string]*stream
	storage           db
	_eventIDs         int64
	heartbeatInterval time.Duration
}

type db interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error)
	Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error
}

func newHandler(db db, heartbeatInterval time.Duration) *handler {
	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           db,
		_eventIDs:         time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
	}
	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := logrus.WithField("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(HttpResError("streaming unsupported", http.StatusBadRequest))
	}
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "private, no-cache, no-transform")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(c.Response(), "\n")
	c.Response().Flush()
	params := c.QueryParams()

	heartbeatType := "legacy"
	if heartbeatParam, exists := params["heartbeat"]; exists && len(heartbeatParam) > 0 {
		heartbeatType = heartbeatParam[0]
	}

	heartbeatMsg, ok := validHeartbeatTypes[heartbeatType]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "invalid heartbeat type. Supported: legacy and message"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}

	var lastEventId int64
	var err error
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "Last-Event-ID should be int"
			log.Error(errorMsg)
			return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	lastEventIdQuery, ok := params["last_event_id"]
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery[0], 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "last_event_id should be int"
			log.Error(errorMsg)
			return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
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
	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))
	session := h.CreateSession(clientIds, lastEventId)

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()
	session.Start()
loop:
	for {
		select {
		case msg, ok := <-session.MessageCh:
			if !ok {
				// can't read from channel, session is closed
				break loop
			}
			_, err = fmt.Fprintf(c.Response(), "event: %v\nid: %v\ndata: %v\n\n", "message", msg.EventId, string(msg.Message))
			if err != nil {
				log.Errorf("msg can't write to connection: %v", err)
				break loop
			}
			c.Response().Flush()

			fromId := "unknown"
			toId := msg.To

			hash := sha256.Sum256(msg.Message)
			messageHash := hex.EncodeToString(hash[:])

			var bridgeMsg datatype.BridgeMessage
			if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
				fromId = bridgeMsg.From
				contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
				messageHash = hex.EncodeToString(contentHash[:])
			}

			logrus.WithFields(logrus.Fields{
				"hash":     messageHash,
				"from":     fromId,
				"to":       toId,
				"event_id": msg.EventId,
				"trace_id": bridgeMsg.TraceId,
			}).Debug("message sent")

			deliveredMessagesMetric.Inc()
			storage.GlobalExpiredCache.MarkDelivered(msg.EventId)
		case <-ticker.C:
			_, err = fmt.Fprint(c.Response(), heartbeatMsg)
			if err != nil {
				log.Errorf("ticker can't write to connection: %v", err)
				break loop
			}
			c.Response().Flush()
		}
	}
	activeConnectionMetric.Dec()
	log.Info("connection closed")
	return nil
}

func (h *handler) SendMessageHandler(c echo.Context) error {
	ctx := c.Request().Context()
	log := logrus.WithContext(ctx).WithField("prefix", "SendMessageHandler")

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

	if config.Config.CopyToURL != "" {
		go func() {
			u, err := url.Parse(config.Config.CopyToURL)
			if err != nil {
				return
			}
			u.RawQuery = params.Encode()
			req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(message))
			if err != nil {
				return
			}
			http.DefaultClient.Do(req) //nolint:errcheck// TODO review golangci-lint issue
		}()
	}
	topic, ok := params["topic"]
	if ok {
		go func(clientID, topic, message string) {
			SendWebhook(clientID, WebhookData{Topic: topic, Hash: message})
		}(clientId[0], topic[0], string(message))
	}

	traceIdParam, ok := params["trace_id"]
	traceId := "unknown"
	if ok {
		uuids, err := uuid.Parse(traceIdParam[0])
		if err != nil {
			log.WithFields(logrus.Fields{
				"error":            err,
				"invalid_trace_id": traceIdParam[0],
			}).Warn("generating a new trace_id")
		} else {
			traceId = uuids.String()
		}
	}
	if traceId == "known" {
		uuids, err := uuid.NewV7()
		if err != nil {
			log.Error(err)
		} else {
			traceId = uuids.String()
		}
	}

	mes, err := json.Marshal(datatype.BridgeMessage{
		From:    clientId[0],
		Message: string(message),
		TraceId: traceId,
	})
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}
	sseMessage := datatype.SseMessage{
		EventId: h.nextID(),
		Message: mes,
		To:      toId[0],
	}
	h.Mux.RLock()
	s, ok := h.Connections[toId[0]]
	h.Mux.RUnlock()
	if ok {
		s.mux.Lock()
		for _, ses := range s.Sessions {
			ses.AddMessageToQueue(ctx, sseMessage)
		}
		s.mux.Unlock()
	}
	go func() {
		log := log.WithField("prefix", "SendMessageHandler.storge.Add")
		err = h.storage.Add(context.Background(), sseMessage, ttl)
		if err != nil {
			// TODO ooops
			log.Errorf("db error: %v", err)
		}
	}()

	var bridgeMsg datatype.BridgeMessage
	fromId := "unknown"

	hash := sha256.Sum256(sseMessage.Message)
	messageHash := hex.EncodeToString(hash[:])

	if err := json.Unmarshal(sseMessage.Message, &bridgeMsg); err == nil {
		fromId = bridgeMsg.From
		contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
		messageHash = hex.EncodeToString(contentHash[:])
	}

	log.WithFields(logrus.Fields{
		"hash":     messageHash,
		"from":     fromId,
		"to":       toId[0],
		"event_id": sseMessage.EventId,
		"trace_id": bridgeMsg.TraceId,
	}).Debug("message received")

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, HttpResOk())

}

func (h *handler) removeConnection(ses *Session) {
	log := logrus.WithField("prefix", "removeConnection")
	log.Infof("remove session: %v", ses.ClientIds)
	for _, id := range ses.ClientIds {
		h.Mux.RLock()
		s, ok := h.Connections[id]
		h.Mux.RUnlock()
		if !ok {
			log.Info("alredy removed")
			continue
		}
		s.mux.Lock()
		for i := range s.Sessions {
			if s.Sessions[i] == ses {
				s.Sessions[i] = s.Sessions[len(s.Sessions)-1]
				s.Sessions = s.Sessions[:len(s.Sessions)-1]
				break
			}
		}
		s.mux.Unlock()

		if len(s.Sessions) == 0 {
			h.Mux.Lock()
			delete(h.Connections, id)
			h.Mux.Unlock()
		}
		activeSubscriptionsMetric.Dec()
	}
}

func (h *handler) CreateSession(clientIds []string, lastEventId int64) *Session {
	log := logrus.WithField("prefix", "CreateSession")
	log.Infof("make new session with ids: %v", clientIds)
	session := NewSession(h.storage, clientIds, lastEventId)
	activeConnectionMetric.Inc()
	for _, id := range clientIds {
		h.Mux.RLock()
		s, ok := h.Connections[id]
		h.Mux.RUnlock()
		if ok {
			s.mux.Lock()
			s.Sessions = append(s.Sessions, session)
			s.mux.Unlock()
		} else {
			h.Mux.Lock()
			h.Connections[id] = &stream{
				mux:      sync.RWMutex{},
				Sessions: []*Session{session},
			}
			h.Mux.Unlock()
		}

		activeSubscriptionsMetric.Inc()
	}
	return session
}

func (h *handler) nextID() int64 {
	return atomic.AddInt64(&h._eventIDs, 1)
}
