package handlerv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
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

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/analytics"
	"github.com/ton-connect/bridge/internal/config"
	handler_common "github.com/ton-connect/bridge/internal/handler"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/utils"
	"github.com/ton-connect/bridge/internal/v1/storage"
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
	uniqueTransferedMessagesNumMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_unique_transfered_messages",
		Help: "The total number of unique transfered_messages",
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

type verifyResponse struct {
	Status string `json:"status"`
}

type stream struct {
	Sessions []*Session
	mux      sync.RWMutex
}
type handler struct {
	Mux               sync.RWMutex
	Connections       map[string]*stream
	storage           storage.Storage
	_eventIDs         int64
	heartbeatInterval time.Duration
	connectionCache   *ConnectionCache
	realIP            *utils.RealIPExtractor
	eventCollector    analytics.EventCollector
	eventBuilder      analytics.EventBuilder
}

func NewHandler(db storage.Storage, heartbeatInterval time.Duration, extractor *utils.RealIPExtractor, collector analytics.EventCollector, builder analytics.EventBuilder) *handler {
	connectionCache := NewConnectionCache(config.Config.ConnectCacheSize, time.Duration(config.Config.ConnectCacheTTL)*time.Second)
	connectionCache.StartBackgroundCleanup(nil)

	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           db,
		_eventIDs:         time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
		connectionCache:   connectionCache,
		realIP:            extractor,
		eventCollector:    collector,
		eventBuilder:      builder,
	}
	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := logrus.WithField("prefix", "EventRegistrationHandler")
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
	_, _ = fmt.Fprint(c.Response(), "\n")
	c.Response().Flush()

	paramsStore, err := handler_common.NewParamsStorage(c, config.Config.MaxBodySize)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		h.logEventRegistrationValidationFailure("", "", "NewParamsStorage error: ")
		return c.JSON(utils.HttpResError(err.Error(), http.StatusBadRequest))
	}

	traceIdParam, ok := paramsStore.Get("trace_id")
	traceId := handler_common.ParseOrGenerateTraceID(traceIdParam, ok)

	heartbeatType := "legacy"
	if heartbeatParam, exists := paramsStore.Get("heartbeat"); exists {
		heartbeatType = heartbeatParam
	}

	heartbeatMsg, ok := validHeartbeatTypes[heartbeatType]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "invalid heartbeat type. Supported: legacy and message"
		log.Error(errorMsg)
		h.logEventRegistrationValidationFailure("", traceId, errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	enableQueueDoneEvent := false
	if queueDoneParam, exists := paramsStore.Get("enable_queue_done_event"); exists && strings.ToLower(queueDoneParam) == "true" {
		enableQueueDoneEvent = true
	}

	var lastEventId int64
	lastEventIDStr := c.Request().Header.Get("Last-Event-ID")
	if lastEventIDStr != "" {
		lastEventId, err = strconv.ParseInt(lastEventIDStr, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "Last-Event-ID should be int"
			log.Error(errorMsg)
			h.logEventRegistrationValidationFailure("", traceId, errorMsg)
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	lastEventIdQuery, ok := paramsStore.Get("last_event_id")
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "last_event_id should be int"
			log.Error(errorMsg)
			h.logEventRegistrationValidationFailure("", traceId, errorMsg)
			return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	clientId, ok := paramsStore.Get("client_id")
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		h.logEventRegistrationValidationFailure("", traceId, errorMsg)
		return c.JSON(utils.HttpResError(errorMsg, http.StatusBadRequest))
	}

	clientIds := strings.Split(clientId, ",")
	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))

	connectIP := h.realIP.Extract(c.Request())
	session := h.CreateSession(clientIds, lastEventId, traceId)

	ip := h.realIP.Extract(c.Request())
	origin := utils.ExtractOrigin(c.Request().Header.Get("Origin"))
	userAgent := c.Request().Header.Get("User-Agent")

	// Store connection in cache
	h.connectionCache.Add(clientId, ip, origin, userAgent)

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session, traceId)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()

	session.Start(heartbeatMsg, enableQueueDoneEvent, h.heartbeatInterval)

	for msg := range session.MessageCh {

		// Parse the message, add BridgeConnectSource, keep it for later logging
		var bridgeMsg models.BridgeMessage
		fromID := "unknown"
		traceID := ""
		messageToSend := msg.Message
		if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
			fromID = bridgeMsg.From
			traceID = bridgeMsg.TraceId
			bridgeMsg.BridgeConnectSource = models.BridgeConnectSource{
				IP: connectIP,
			}
			if modifiedMessage, err := json.Marshal(bridgeMsg); err == nil {
				messageToSend = modifiedMessage
			}
		}

		var sseMessage string
		if msg.EventId == -1 {
			sseMessage = string(messageToSend)
		} else {
			sseMessage = fmt.Sprintf("event: message\r\nid: %v\r\ndata: %v\r\n\r\n", msg.EventId, string(messageToSend))
		}

		_, err = fmt.Fprint(c.Response(), sseMessage)
		if err != nil {
			log.Errorf("msg can't write to connection: %v", err)
			break
		}
		c.Response().Flush()
		if msg.EventId != -1 {
			hash := sha256.Sum256(messageToSend)
			messageHash := hex.EncodeToString(hash[:])

			logrus.WithFields(logrus.Fields{
				"hash":     messageHash,
				"from":     fromID,
				"to":       msg.To,
				"event_id": msg.EventId,
				"trace_id": traceID,
			}).Debug("message sent")

			if h.eventCollector != nil {
				_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageSentEvent(
					msg.To,
					traceID,
					msg.EventId,
					messageHash,
				))
			}
			deliveredMessagesMetric.Inc()
			storage.ExpiredCache.Mark(msg.EventId)
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
		log.Error(errorMsg)
		return h.logMessageSentValidationFailure(c, errorMsg, "", traceId, "", "")
	}
	clientID := clientIdValues[0]

	toId, ok := params["to"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"to\" not present"
		log.Error(errorMsg)
		return h.logMessageSentValidationFailure(c, errorMsg, clientID, traceId, "", "")
	}

	ttlParam, ok := params["ttl"]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" not present"
		log.Error(errorMsg)
		return h.logMessageSentValidationFailure(c, errorMsg, clientID, traceId, "", "")
	}
	ttl, err := strconv.ParseInt(ttlParam[0], 10, 32)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return h.logMessageSentValidationFailure(c, err.Error(), clientID, traceId, "", "")
	}
	if ttl > 300 { // TODO: config
		badRequestMetric.Inc()
		errorMsg := "param \"ttl\" too high"
		log.Error(errorMsg)
		return h.logMessageSentValidationFailure(c, errorMsg, clientID, traceId, "", "")
	}
	message, err := io.ReadAll(c.Request().Body)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return h.logMessageSentValidationFailure(c, err.Error(), clientID, traceId, "", "")
	}

	data := append(message, []byte(clientID)...)
	sum := sha256.Sum256(data)
	messageId := int64(binary.BigEndian.Uint64(sum[:8]))
	if ok := storage.TransferedCache.MarkIfNotExists(messageId); ok {
		uniqueTransferedMessagesNumMetric.Inc()
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
		}(clientID, topic, string(message))
	}

	var requestSource string
	noRequestSourceParam, ok := params["no_request_source"]
	enableRequestSource := !ok || len(noRequestSourceParam) == 0 || strings.ToLower(noRequestSourceParam[0]) != "true"

	if enableRequestSource {
		origin := utils.ExtractOrigin(c.Request().Header.Get("Origin"))
		ip := h.realIP.Extract(c.Request())
		userAgent := c.Request().Header.Get("User-Agent")

		encryptedRequestSource, err := utils.EncryptRequestSourceWithWalletID(
			models.BridgeRequestSource{
				Origin:    origin,
				IP:        ip,
				Time:      strconv.FormatInt(time.Now().Unix(), 10),
				UserAgent: userAgent,
			},
			toId[0], // todo - check to id properly
		)
		if err != nil {
			badRequestMetric.Inc()
			log.Error(err)
			return h.logMessageSentValidationFailure(
				c,
				fmt.Sprintf("failed to encrypt request source: %v", err),
				clientID,
				traceId,
				topic,
				"",
			)
		}
		requestSource = encryptedRequestSource
	}

	mes, err := json.Marshal(models.BridgeMessage{
		From:                clientID,
		Message:             string(message),
		BridgeRequestSource: requestSource,
		TraceId:             traceId,
	})
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return h.logMessageSentValidationFailure(c, err.Error(), clientID, traceId, topic, "")
	}

	if topic == "disconnect" && len(mes) < config.Config.DisconnectEventMaxSize {
		ttl = config.Config.DisconnectEventsTTL
	}

	sseMessage := models.SseMessage{
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

	var bridgeMsg models.BridgeMessage
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
		"trace_id": traceId,
	}).Debug("message received")

	if h.eventCollector != nil {
		_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageReceivedEvent(
			clientID,
			traceId,
			topic,
			sseMessage.EventId,
			messageHash,
		))
	}

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, utils.HttpResOk())

}

func (h *handler) ConnectVerifyHandler(c echo.Context) error {
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
	url, ok := paramsStore.Get("url")
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
		status := h.connectionCache.Verify(clientId, ip, utils.ExtractOrigin(url))
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
				"param \"type\" must be one of: connect, message",
			))
		}
		return c.JSON(utils.HttpResError("param \"type\" must be one of: connect, message", http.StatusBadRequest))
	}
}

func (h *handler) removeConnection(ses *Session, traceID string) {
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
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeEventsClientUnsubscribedEvent(id, traceID))
		}
	}
}

func (h *handler) CreateSession(clientIds []string, lastEventId int64, traceId string) *Session {
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
		if h.eventCollector != nil {
			_ = h.eventCollector.TryAdd(h.eventBuilder.NewBridgeEventsClientSubscribedEvent(id, traceId))
		}
	}
	return session
}

func (h *handler) nextID() int64 {
	return atomic.AddInt64(&h._eventIDs, 1)
}

func (h *handler) logEventRegistrationValidationFailure(clientID, traceID, errorMsg string) {
	if h.eventCollector == nil {
		return
	}
	h.eventCollector.TryAdd(h.eventBuilder.NewBridgeMessageValidationFailedEvent(
		clientID,
		traceID,
		"",
		errorMsg,
	))
}

func (h *handler) logMessageSentValidationFailure(
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
