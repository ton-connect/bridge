package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	expiredMessagesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "number_of_expired_messages",
		Help: "The total number of expired messages",
	})
	expiredMessagesCacheSizeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "expired_messages_cache_size",
		Help: "The current size of the expired messages cache",
	})
	transferedMessagesCacheSizeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "transfered_messages_cache_size",
		Help: "The current size of the transfered messages cache",
	})
)
