package main

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

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/config"
	"github.com/tonkeeper/bridge/datatype"
	"github.com/tonkeeper/bridge/storage"
	"github.com/tonkeeper/bridge/tonmetrics"
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
	storage           db
	_eventIDs         int64
	heartbeatInterval time.Duration
	connectionCache   *ConnectionCache
	realIP            *realIPExtractor
	analytics         tonmetrics.AnalyticsClient
}

type db interface {
	GetMessages(ctx context.Context, keys []string, lastEventId int64) ([]datatype.SseMessage, error)
	Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error
}

func newHandler(db db, heartbeatInterval time.Duration, extractor *realIPExtractor) *handler {
	connectionCache := NewConnectionCache(config.Config.ConnectCacheSize, time.Duration(config.Config.ConnectCacheTTL)*time.Second)
	connectionCache.StartBackgroundCleanup()

	h := handler{
		Mux:               sync.RWMutex{},
		Connections:       make(map[string]*stream),
		storage:           db,
		_eventIDs:         time.Now().UnixMicro(),
		heartbeatInterval: heartbeatInterval,
		connectionCache:   connectionCache,
		realIP:            extractor,
		analytics:         tonmetrics.NewAnalyticsClient(),
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

	paramsStore, err := NewParamsStorage(c, config.Config.MaxBodySize)
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}

	heartbeatType := "legacy"
	if heartbeatParam, exists := paramsStore.Get("heartbeat"); exists {
		heartbeatType = heartbeatParam
	}

	heartbeatMsg, ok := validHeartbeatTypes[heartbeatType]
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "invalid heartbeat type. Supported: legacy and message"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
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
			return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	lastEventIdQuery, ok := paramsStore.Get("last_event_id")
	if ok && lastEventId == 0 {
		lastEventId, err = strconv.ParseInt(lastEventIdQuery, 10, 64)
		if err != nil {
			badRequestMetric.Inc()
			errorMsg := "last_event_id should be int"
			log.Error(errorMsg)
			return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
		}
	}
	clientId, ok := paramsStore.Get("client_id")
	if !ok {
		badRequestMetric.Inc()
		errorMsg := "param \"client_id\" not present"
		log.Error(errorMsg)
		return c.JSON(HttpResError(errorMsg, http.StatusBadRequest))
	}
	clientIds := strings.Split(clientId, ",")
	clientIdsPerConnectionMetric.Observe(float64(len(clientIds)))

	connectIP := h.realIP.Extract(c.Request())
	session := h.CreateSession(clientIds, lastEventId)

	ip := h.realIP.Extract(c.Request())
	origin := ExtractOrigin(c.Request().Header.Get("Origin"))
	userAgent := c.Request().Header.Get("User-Agent")

	// Store connection in cache
	h.connectionCache.Add(clientId, ip, origin, userAgent)

	ctx := c.Request().Context()
	notify := ctx.Done()
	go func() {
		<-notify
		close(session.Closer)
		h.removeConnection(session)
		log.Infof("connection: %v closed with error %v", session.ClientIds, ctx.Err())
	}()

	session.Start(heartbeatMsg, enableQueueDoneEvent, h.heartbeatInterval)

	for msg := range session.MessageCh {

		// Parse the message, add BridgeConnectSource, keep it for later logging
		var bridgeMsg datatype.BridgeMessage
		messageToSend := msg.Message
		if err := json.Unmarshal(msg.Message, &bridgeMsg); err == nil {
			bridgeMsg.BridgeConnectSource = datatype.BridgeConnectSource{
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
				"from":     bridgeMsg.From,
				"to":       msg.To,
				"event_id": msg.EventId,
				"trace_id": bridgeMsg.TraceId,
			}).Debug("message sent")

			go h.analytics.SendEvent(tonmetrics.CreateBridgeRequestReceivedEvent(
				config.Config.BridgeURL,
				msg.To,
				bridgeMsg.TraceId,
				config.Config.Environment,
				config.Config.BridgeVersion,
				config.Config.NetworkId,
				msg.EventId,
			))

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

	data := append(message, []byte(clientId[0])...)
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
			SendWebhook(clientID, WebhookData{Topic: topic, Hash: message})
		}(clientId[0], topic, string(message))
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
	if traceId == "unknown" {
		uuids, err := uuid.NewV7()
		if err != nil {
			log.Error(err)
		} else {
			traceId = uuids.String()
		}
	}

	var requestSource string
	noRequestSourceParam, ok := params["no_request_source"]
	enableRequestSource := !ok || len(noRequestSourceParam) == 0 || strings.ToLower(noRequestSourceParam[0]) != "true"

	if enableRequestSource {
		origin := ExtractOrigin(c.Request().Header.Get("Origin"))
		ip := h.realIP.Extract(c.Request())
		userAgent := c.Request().Header.Get("User-Agent")

		encryptedRequestSource, err := encryptRequestSourceWithWalletID(
			datatype.BridgeRequestSource{
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
			return c.JSON(HttpResError(fmt.Sprintf("failed to encrypt request source: %v", err), http.StatusBadRequest))
		}
		requestSource = encryptedRequestSource
	}

	mes, err := json.Marshal(datatype.BridgeMessage{
		From:                clientId[0],
		Message:             string(message),
		BridgeRequestSource: requestSource,
		TraceId:             traceId,
	})
	if err != nil {
		badRequestMetric.Inc()
		log.Error(err)
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}

	if topic == "disconnect" && len(mes) < config.Config.DisconnectEventMaxSize {
		ttl = config.Config.DisconnectEventsTTL
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

	go h.analytics.SendEvent(tonmetrics.CreateBridgeRequestSentEvent(
		config.Config.BridgeURL,
		clientId[0],
		traceId,
		topic,
		config.Config.Environment,
		config.Config.BridgeVersion,
		config.Config.NetworkId,
		sseMessage.EventId,
	))

	transferedMessagesNumMetric.Inc()
	return c.JSON(http.StatusOK, HttpResOk())

}

func (h *handler) ConnectVerifyHandler(c echo.Context) error {
	ip := h.realIP.Extract(c.Request())
	userAgent := c.Request().Header.Get("User-Agent")

	paramsStore, err := NewParamsStorage(c, config.Config.MaxBodySize)
	if err != nil {
		badRequestMetric.Inc()
		return c.JSON(HttpResError(err.Error(), http.StatusBadRequest))
	}

	clientId, ok := paramsStore.Get("client_id")
	if !ok {
		badRequestMetric.Inc()
		return c.JSON(HttpResError("param \"client_id\" not present", http.StatusBadRequest))
	}
	url, ok := paramsStore.Get("url")
	if !ok {
		badRequestMetric.Inc()
		return c.JSON(HttpResError("param \"url\" not present", http.StatusBadRequest))
	}
	qtype, ok := paramsStore.Get("type")
	if !ok {
		qtype = "connect"
	}

	// Default status
	status := "unknown"

	switch strings.ToLower(qtype) {
	case "connect":
		if res := h.connectionCache.Verify(clientId, ip, ExtractOrigin(url), userAgent); res == "ok" {
			status = "ok"
		}
	default:
		badRequestMetric.Inc()
		return c.JSON(HttpResError("param \"type\" must be one of: connect, message", http.StatusBadRequest))
	}
	return c.JSON(http.StatusOK, verifyResponse{Status: status})
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
