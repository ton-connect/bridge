package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gammazero/deque"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
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
	SessionKill chan SessionChan
}

func newHandler() *handler {

	h := handler{
		Mux:         sync.Mutex{},
		Connections: make(map[string]*Session),
		SessionKill: make(chan SessionChan, 10),
	}

	go h.SessionRemover()
	return &h
}

func (h *handler) EventRegistrationHandler(c echo.Context) error {
	log := log.WithField("prefix", "EventRegistrationHandler")
	_, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		http.Error(c.Response().Writer, "streaming unsupported", http.StatusInternalServerError)
		return c.JSON(HttpResError("streaming unsupported", http.StatusBadRequest))
	}
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding")
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
	newSession := NewSession(clientId[0], &c, len(clientIds), h.SessionKill)
	for _, id := range clientIds {
		oldSes, ok := h.Connections[id]
		if ok { // remove old connection
			oldSes.mux.Lock()
			oldSes.Subscribers--
			activeSubscriptionsMetric.Dec()
			if oldSes.Subscribers < 1 {
				if oldSes.Connection != nil {
					oldSes.Connection = nil
					activeConnectionMetric.Dec()
				}
			}
			messages := deque.New[MessageWithTtl]()

			for oldSes.MessageQueue.Len() != 0 {
				m := oldSes.MessageQueue.PopFront()
				if m.To == id {
					newSession.mux.Lock()
					newSession.MessageQueue.PushBack(m)
					newSession.mux.Unlock()
				} else {
					messages.PushBack(m)
				}

			}
			oldSes.MessageQueue = messages
			oldSes.mux.Unlock()
		}
		h.Mux.Lock()
		h.Connections[id] = newSession
		h.Mux.Unlock()
		activeSubscriptionsMetric.Inc()
	}
	activeConnectionMetric.Inc()
	notify := c.Request().Context().Done()
	go func() {
		<-notify
		newSession.mux.Lock()
		newSession.Connection = nil
		newSession.mux.Unlock()
		activeConnectionMetric.Dec()
		log.Infof("remove connection with clientId: %v from map", clientId[0])
	}()

	for {
		msg, open := <-newSession.MessageCh
		if !open {
			break
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.Encode(msg)
		fmt.Fprintf(c.Response(), "data: %v\n\n", buf.String())
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
	toIdSession, ok := h.Connections[toId[0]]
	if !ok {
		newSession := NewSession(toId[0], nil, 0, h.SessionKill)
		h.Connections[toId[0]] = newSession
		toIdSession = newSession
		activeSubscriptionsMetric.Inc()
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
	toIdSession.addMessageToDeque(clientId[0], toId[0], ttl, message, done, remove)
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

func (h *handler) SessionRemover() {
	log := log.WithField("prefix", "SessionRemover")
	for {
		sn := <-h.SessionKill
		for _, id := range sn.Ids {
			ses, ok := h.Connections[id]
			if ok && ses.SessionId == sn.SessionId {
				h.Mux.Lock()
				delete(h.Connections, id)
				h.Mux.Unlock()
				activeSubscriptionsMetric.Dec()
				log.Infof("remove connection: %v", id)
			}
		}

	}
}
