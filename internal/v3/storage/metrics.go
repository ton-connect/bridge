package storagev3

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	DropPathReplay = "replay"
	DropPathLive   = "live"
	DropPathSSE    = "sse"

	DropReasonSubscriberFull = "subscriber_full"
	DropReasonWriteError     = "write_error"
)

// MessagesDropped counts messages discarded on the way to a connected subscriber.
var MessagesDropped = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "bridge_messages_dropped_total",
	Help: "Messages discarded on the way to a connected subscriber, by delivery path and reason",
}, []string{"path", "reason"})

func init() {
	// Every series exists from startup, so the first drop reads as growth rather than as a new series.
	MessagesDropped.WithLabelValues(DropPathReplay, DropReasonSubscriberFull)
	MessagesDropped.WithLabelValues(DropPathLive, DropReasonSubscriberFull)
	MessagesDropped.WithLabelValues(DropPathSSE, DropReasonWriteError)
}
