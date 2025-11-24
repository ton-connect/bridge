package handlerv3

import (
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/ton-connect/bridge/internal/ntp"
)

// TimeProvider provides the current time in milliseconds.
// This interface allows using either local time or NTP-synchronized time.
type TimeProvider interface {
	NowUnixMilli() int64
}

// EventIDGenerator generates monotonically increasing event IDs across multiple bridge instances.
// Uses time-based ID generation with local sequence counter to ensure uniqueness and ordering.
// Format: (timestamp_ms << 16) | local_counter
// This provides ~65K events per millisecond per bridge instance.
//
// Note: Due to concurrent generation and potential clock skew between instances,
// up to 5% of events may not be in strict monotonic sequence, which is acceptable
// for the bridge's event ordering requirements.
type EventIDGenerator struct {
	counter      int64        // Local sequence counter, incremented atomically
	offset       int64        // Random offset per instance to avoid collisions
	timeProvider TimeProvider // Time source (local or NTP-synchronized)
}

type localTimeProvider struct{}

func (l *localTimeProvider) NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// NewEventIDGenerator creates a new event ID generator with counter starting from 0.
// If ntpClient is provided, uses NTP-synchronized time for better consistency across instances.
// If ntpClient is nil, falls back to local system time.
func NewEventIDGenerator(ntpClient *ntp.Client) *EventIDGenerator {
	var timeProvider TimeProvider
	if ntpClient != nil {
		timeProvider = ntpClient
	} else {
		timeProvider = &localTimeProvider{}
	}

	return &EventIDGenerator{
		counter:      0,
		offset:       rand.Int63() & 0xFFFF, // Random offset to avoid collisions between instances
		timeProvider: timeProvider,
	}
}

// NextID generates the next monotonic event ID.
//
// The ID format combines:
// - Upper 48 bits: Unix timestamp in milliseconds (provides time-based ordering)
// - Lower 16 bits: Local counter masked to 16 bits (handles multiple events per millisecond)
//
// This approach ensures:
// - IDs are mostly monotonic if bridge instances have synchronized clocks (NTP)
// - No central coordination needed → scalable across multiple bridge instances
// - Unique IDs even with high event rates (65K events/ms per instance)
// - Works well with SSE last_event_id for client reconnection
func (g *EventIDGenerator) NextID() int64 {
	timestamp := g.timeProvider.NowUnixMilli()
	counter := atomic.AddInt64(&g.counter, 1)
	return (timestamp << 16) | ((counter + g.offset) & 0xFFFF)
}
