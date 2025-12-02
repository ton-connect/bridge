package handlerv3

import (
	"math/rand"
	"sync/atomic"

	"github.com/ton-connect/bridge/internal/ntp"
)

// EventIDGenerator generates monotonically increasing event IDs across multiple bridge instances.
// Uses time-based ID generation with local sequence counter to ensure uniqueness and ordering.
// Format: (timestamp_ms << 11) | local_counter (53 bits total for JavaScript compatibility)
// This provides ~2K events per millisecond per bridge instance.
//
// Note: Due to concurrent generation and potential clock skew between instances,
// up to 5% of events may not be in strict monotonic sequence, which is acceptable
// for the bridge's event ordering requirements.
type EventIDGenerator struct {
	counter      int64            // Local sequence counter, incremented atomically
	offset       int64            // Random offset per instance to avoid collisions (11 bits)
	timeProvider ntp.TimeProvider // Time source (local or NTP-synchronized)
}

// NewEventIDGenerator creates a new event ID generator with counter starting from 0.
// The timeProvider parameter determines the time source:
// - Use ntp.Client for NTP-synchronized time (better consistency across bridge instances)
// - Use ntp.LocalTimeProvider for local system time (fallback when NTP is unavailable)
func NewEventIDGenerator(timeProvider ntp.TimeProvider) *EventIDGenerator {
	return &EventIDGenerator{
		counter:      0,
		offset:       rand.Int63() & 0x7FF, // Random offset (11 bits) to avoid collisions between instances
		timeProvider: timeProvider,
	}
}

// NextID generates the next monotonic event ID.
//
// The ID format combines (53 bits total for JavaScript Number.MAX_SAFE_INTEGER compatibility):
// - Upper 42 bits: Unix timestamp in milliseconds (supports dates up to year 2100)
// - Lower 11 bits: Local counter masked to 11 bits (handles multiple events per millisecond)
//
// This approach ensures:
// - IDs are mostly monotonic if bridge instances have synchronized clocks (NTP)
// - No central coordination needed → scalable across multiple bridge instances
// - Unique IDs even with high event rates (~2K events/ms per instance)
// - Works well with SSE last_event_id for client reconnection
// - Safe integer representation in JavaScript (< 2^53)
func (g *EventIDGenerator) NextID() int64 {
	timestamp := g.timeProvider.NowUnixMilli()
	counter := atomic.AddInt64(&g.counter, 1)
	return getIdFromParams(timestamp, counter+g.offset)
}

func getIdFromParams(timestamp int64, nonce int64) int64 {
	return ((timestamp << 11) | (nonce & 0x7FF)) & 0x1FFFFFFFFFFFFF
}
