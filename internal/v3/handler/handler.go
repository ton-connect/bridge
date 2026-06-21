package handlerv3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/config"
	handler_common "github.com/ton-connect/bridge/internal/handler"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/ntp"
	"github.com/ton-connect/bridge/internal/utils"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
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
	storage           storagev3.Storage
	eventIDGen        *EventIDGenerator
	heartbeatInterval time.Duration
	realIP            *utils.RealIPExtractor
	eventCollector    analytics.EventCollector
	eventBuilder      analytics.EventBuilder
}

func NewHandler(s storagev3.Storage, heartbeatInterval time.Duration, extractor *utils.RealIPExtractor, timeProvider ntp.TimeProvider, collector analytics.EventCollector, builder analytics.EventBuilder) *handler {
	// TODO support extractor in v3
	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           s,
		eventIDGen:        NewEventIDGenerator(timeProvider),
		realIP:            extractor,
		heartbeatInterval: heartbeatInterval,
		eventCollector:    collector,
		eventBuilder:      builder,
	}
	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	logger := slog.With("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(utils.HttpResError("streaming unsupported", http.StatusBadRequest))
	}
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "private, no-cache, no-transform")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Transfer-Encoding", "chunked")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(c.Response(), "\n"); err != nil {
		logger.Error("failed to write initial newline", "err", err)
		return err
	}
	c.Response().Flush()
	params := c.QueryParams()

	traceIdParam, ok := params["trace_id"]
	traceIdValue := ""
	if ok && len(traceIdParam) > 0 {
		traceIdValue = traceIdParam[0]
	}
	traceId := handler_common.ParseOrGenerateTraceID(traceIdValue, ok && len(traceIdParam) > 0)

	heartbeatType := "legacy"
	if heartbeatParam, exists := params["heartbeat"]; exists && len(heartbeatParam) > 0 {
		heartbeatType = heartbeatParam[0]
	}

	heartbeatMsg, ok := validHeartbeatTypes[heartbeatType]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "invalid heartbeat type. Supported: legacy and message"
		logger.Error(errorMsg)
		h.logEventRegistrationValidationFailure("", traceId, "events/heartbeat")
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	var lastEventId int64
	var err error
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "Last-Event-ID should be int"
			logger.Error(errorMsg)
			h.logEventRegistrationValidationFailure("", traceId, "events/last-event-id-header")
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	lastEventIdQuery, ok := params["last_event_id"]
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery[0], 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "last_event_id should be int"
			logger.Error(errorMsg)
			h.logEventRegistrationValidationFailure("", traceId, "events/last-event-id-query")
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	clientId, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		logger.Error(errorMsg)
		h.logEventRegistrationValidationFailure("", traceId, "events/missing-client-id")
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	clientIds := strings.Split(clientId[0], ",")
	for _, id := range clientIds {
		if _, err := utils.NewPublicAddressFromString(id); err != nil {
			badRequestMetric.Inc()
			errMsg := fmt.Errorf("param \"client_id\" must be a valid public address, error: %w", err).Error()
			logger.Error(errMsg)
			h.logEventRegistrationValidationFailure("", traceId, errMsg)
			return c.JSON(utils.HttpResError(errMsg, http.StatusBadRequest))
		}
	}
	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))

	session := h.CreateSession(clientIds, lastEventId, traceId)

	// Track connection for verification
	if len(clientIds) > 0 {
		ip := h.realIP.Extract(c.Request())
		origin := utils.ExtractOrigin(c.Request().Header.Get("Origin"))
		userAgent := c.Request().Header.Get("User-Agent")

		conn := storagev3.ConnectionInfo{
			ClientID:  clientIds[0],
			IP:        ip,
			Origin:    origin,
			UserAgent: userAgent,
		}

		ttl := time.Duration(config.Config.ConnectCacheTTL) * time.Second
		if err := h.storage.AddConnection(c.Request().Context(), conn, ttl); err != nil {
			logger.Warn("failed to store connection", "err", err)
		}
	}

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		session.Close()
		h.removeConnection(session, traceId)
		logger.Info("connection closed with error", "client_ids", session.ClientIds, "err", ctx.Err())
	}()

	// Force-close the session after max lifetime. This runs outside the select loop
	// so it works even when messages are flowing continuously.
	maxLifetime := sseMaxLifetimeWithJitter()
	lifetimeTimer := time.AfterFunc(maxLifetime, func() {
		logger.Info("SSE connection max lifetime reached, closing", "max_lifetime", maxLifetime)
		session.Close()
	})
	defer lifetimeTimer.Stop()

	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()

	session.Start()
loop:
	for {
		select {
		case msg, ok := <-session.GetMessages():
			if !ok {
				// can't read from channel, session is closed
				break loop
			}
			_, err = fmt.Fprintf(c.Response(), "event: %v\nid: %v\ndata: %v\n\n", "message", msg.EventId, string(msg.Message))
			if err != nil {
				logger.Error("msg can't write to connection", "err", err)
				break loop
			}
			c.Response().Flush()

			fromId := "unknown"
			toId := msg.To
			traceID := ""

			hash := sha256.Sum256(msg.Message)
			messageHash := hex.EncodeToString(hash[:])

			var bridgeMsg models.BridgeMessage
			if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
				fromId = bridgeMsg.From
				traceID = bridgeMsg.TraceId
				contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
				messageHash = hex.EncodeToString(contentHash[:])
			}

			slog.Debug("message sent", "hash", messageHash, "from", fromId, "to", toId, "event_id", msg.EventId, "trace_id", traceID)

			if h.eventCollector != nil {
				_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageSentEvent(
					msg.To,
					traceID,
					msg.EventId,
					messageHash,
				))
			}
			deliveredMessagesMetric.Inc()
			storagev3.ExpiredCache.Mark(msg.EventId)
		case <-ticker.C:
			_, err = fmt.Fprint(c.Response(), heartbeatMsg)
			if err != nil {
				logger.Error("ticker can't write heartbeat to connection", "err", err)
			}
			c.Response().Flush()
		}
	}
	activeConnectionMetric.Dec()
	logger.Info("connection closed")
	return nil
}

func (h *handler) SendMessageHandler(c echo.Context) error {
	logger := slog.With("prefix", "SendMessageHandler")

	params := c.QueryParams()

	traceIdParam, ok := params["trace_id"]
	traceIdValue := ""
	if ok && len(traceIdParam) > 0 {
		traceIdValue = traceIdParam[0]
	}
	traceId := handler_common.ParseOrGenerateTraceID(traceIdValue, ok && len(traceIdParam) > 0)

	clientIdValues, ok := params["client_id"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		logger.Error(errorMsg)
		return h.failValidation(c, errorMsg, "", traceId, "", "")
	}

	clientID, err := utils.NewPublicAddressFromString(clientIdValues[0])
	if err != nil {
		err = fmt.Errorf("failed to parse the \"client_id\" address: %w", err)
		badRequestMetric.Inc()
		logger.Error(err.Error())
		return h.failValidation(c, err.Error(), clientIdValues[0], traceId, "", "")
	}

	toIdValues, ok := params["to"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"to\" not present"
		logger.Error(errorMsg)
		return h.failValidation(c, errorMsg, clientID.String(), traceId, "", "")
	}

	toId, err := utils.NewPublicAddressFromString(toIdValues[0])
	if err != nil {
		err = fmt.Errorf("failed to parse the \"to\" address: %w", err)
		badRequestMetric.Inc()
		logger.Error(err.Error())
		return h.failValidation(c, err.Error(), clientID.String(), traceId, "", "")
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" not present"
		logger.Error(errorMsg)
		return h.failValidation(c, errorMsg, clientID.String(), traceId, "", "")
	}
	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		badRequestMetric.Inc()
		logger.Error(err.Error())
		return h.failValidation(c, err.Error(), clientID.String(), traceId, "", "")
	}
	if ttl > 300 { // TODO: config MaxTTL value
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" too high"
		logger.Error(errorMsg)
		return h.failValidation(c, errorMsg, clientID.String(), traceId, "", "")
	}
	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		badRequestMetric.Inc()
		logger.Error(err.Error())
		return h.failValidation(c, err.Error(), clientID.String(), traceId, "", "")
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
	topicParam, ok := params["topic"]
	topic := ""
	if ok {
		topic = topicParam[0]
		go func(clientID, topic, message string) {
			handler_common.SendWebhook(clientID, handler_common.WebhookData{Topic: topic, Hash: message})
		}(clientID.String(), topic, string(message))
	}

	mes, err := json.Marshal(models.BridgeMessage{
		From:    clientID.String(),
		Message: string(message),
		TraceId: traceId,
	})
	if err != nil {
		badRequestMetric.Inc()
		logger.Error(err.Error())
		return h.failValidation(c, err.Error(), clientID.String(), traceId, topic, "")
	}

	sseMessage := models.SseMessage{
		EventId: h.eventIDGen.NextID(),
		Message: mes,
		To:      toId.String(),
	}

	// Send message only to storage - pub-sub will handle distribution
	go func() {
		logger := logger.With("prefix", "SendMessageHandler.storage.Pub")
		err = h.storage.Pub(context.Background(), sseMessage, ttl)
		if err != nil {
			// TODO ooops
			logger.Error("db error", "err", err)
		}
	}()

	var bridgeMsg models.BridgeMessage
	fromId := "unknown"

	hash := sha256.Sum256(sseMessage.Message)
	messageHash := hex.EncodeToString(hash[:])

	if err := json.Unmarshal(sseMessage.Message, &bridgeMsg); err == nil {
		fromId = bridgeMsg.From
		contentHash := sha256.Sum256([]byte(bridgeMsg.Message))
		messageHash = hex.EncodeToString(contentHash[:])
	}

	logger.Debug("message received", "hash", messageHash, "from", fromId, "to", toId[0], "event_id", sseMessage.EventId, "trace_id", traceId)

	if h.eventCollector != nil {
		_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageReceivedEvent(
			clientID.String(),
			traceId,
			topic,
			sseMessage.EventId,
			messageHash,
		))
	}

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, utils.HttpResOk())
}

type verifyResponse struct {
	Status string `json:"status"`
}

func (h *handler) ConnectVerifyHandler(c echo.Context) error {
	ctx := c.Request().Context()
	ip := h.realIP.Extract(c.Request())

	paramsStore, err := handler_common.NewParamsStorage(c, config.Config.MaxBodySize)
	if err != nil {
		badRequestMetric.Inc()
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyValidationFailedEvent(
				"",
				"",
				http.StatusBadRequest,
				err.Error(),
			))
		}
		return c.JSON(utils.HttpResError(err.Error(), http.StatusBadRequest))
	}

	traceIdParam, ok := paramsStore.Get("trace_id")
	traceId := handler_common.ParseOrGenerateTraceID(traceIdParam, ok)

	clientId, ok := paramsStore.Get("client_id")
	if !ok {
		badRequestMetric.Inc()
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyValidationFailedEvent(
				"",
				traceId,
				http.StatusBadRequest,
				"param \"client_id\" not present",
			))
		}
		return c.JSON(utils.HttpResError("param \"client_id\" not present", http.StatusBadRequest))
	}
	urlParam, ok := paramsStore.Get("url")
	if !ok {
		badRequestMetric.Inc()
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyValidationFailedEvent(
				clientId,
				traceId,
				http.StatusBadRequest,
				"param \"url\" not present",
			))
		}
		return c.JSON(utils.HttpResError("param \"url\" not present", http.StatusBadRequest))
	}
	qtype, ok := paramsStore.Get("type")
	if !ok {
		qtype = "connect"
	}

	switch strings.ToLower(qtype) {
	case "connect":
		conn := storagev3.ConnectionInfo{
			ClientID: clientId,
			IP:       ip,
			Origin:   utils.ExtractOrigin(urlParam),
		}
		status, err := h.storage.VerifyConnection(ctx, conn)
		if err != nil {
			if h.eventCollector != nil {
				_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyValidationFailedEvent(
					clientId,
					traceId,
					http.StatusInternalServerError,
					err.Error(),
				))
			}
			return c.JSON(utils.HttpResError(err.Error(), http.StatusInternalServerError))
		}
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyEvent(clientId, traceId, status))
		}
		return c.JSON(http.StatusOK, verifyResponse{Status: status})
	default:
		badRequestMetric.Inc()
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeVerifyValidationFailedEvent(
				clientId,
				traceId,
				http.StatusBadRequest,
				"param \"type\" must be: connect",
			))
		}
		return c.JSON(utils.HttpResError("param \"type\" must be: connect", http.StatusBadRequest))
	}
}

func (h *handler) removeConnection(ses *Session, traceID string) {
	logger := slog.With("prefix", "removeConnection")
	logger.Info("remove session", "client_ids", ses.ClientIds)
	for _, id := range ses.ClientIds {
		h.Mux.RLock()
		s, ok := h.Connections[id]
		h.Mux.RUnlock()
		if !ok {
			logger.Info("alredy removed")
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
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeEventsClientUnsubscribedEvent(id, traceID))
		}
	}
}

func (h *handler) CreateSession(clientIds []string, lastEventId int64, traceID string) *Session {
	logger := slog.With("prefix", "CreateSession")
	logger.Info("make new session", "client_ids", clientIds)
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
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeEventsClientSubscribedEvent(id, traceID))
		}
	}
	return session
}

func (h *handler) logEventRegistrationValidationFailure(clientID, traceID, requestType string) {
	if h.eventCollector == nil {
		return
	}
	h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageValidationFailedEvent(
		clientID,
		traceID,
		requestType,
		"",
	))
}

func (h *handler) failValidation(
	c echo.Context,
	msg string,
	clientID string,
	traceID string,
	topic string,
	messageHash string,
) error {
	if h.eventCollector != nil {
		_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageValidationFailedEvent(
			clientID,
			traceID,
			topic,
			messageHash,
		))
	}
	return c.JSON(utils.HttpResError(msg, http.StatusBadRequest))
}

// sseMaxLifetimeWithJitter returns the configured SSE max lifetime plus a random jitter.
func sseMaxLifetimeWithJitter() time.Duration {
	base := time.Duration(config.Config.SSEMaxLifetime) * time.Second
	jitterMax := config.Config.SSEMaxLifetimeJitter
	if jitterMax <= 0 {
		return base
	}
	jitterSeconds := rand.Int63n(jitterMax)
	return base + time.Duration(jitterSeconds)*time.Second
}
