package antiscam

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// Service provides antiscam filtering for HTTP handlers.
type Service struct {
	checker DomainChecker
}

// NewService creates an antiscam Service backed by the given DomainChecker.
func NewService(checker DomainChecker) *Service {
	return &Service{checker: checker}
}

// CheckPush returns true (and logs + increments metrics) if the origin is blocked.
// The caller should return 200 OK to the client without delivering the message.
func (s *Service) CheckPush(origin, traceId string) bool {
	if !s.checker.IsBlocked(origin) {
		return false
	}
	BlockedPushesMetric.Inc()
	logrus.WithFields(logrus.Fields{"origin": origin, "trace_id": traceId}).
		Info("message silently dropped by antiscam filter")
	return true
}

// CheckSSE returns true if the origin is blocked.
// When true the caller should invoke WritePoisonStream instead of creating a real session.
func (s *Service) CheckSSE(origin, traceId string) bool {
	if !s.checker.IsBlocked(origin) {
		return false
	}
	PoisonedConnectionsMetric.Inc()
	logrus.WithFields(logrus.Fields{"origin": origin, "trace_id": traceId}).
		Info("SSE connection poisoned by antiscam filter")
	return true
}

// WritePoisonStream sends random hex-encoded garbage as SSE events at random
// 2–15 s intervals until the done channel is closed or the writer fails.
// It uses the raw http.ResponseWriter + http.Flusher so it does not depend on
// any particular web framework.
func (s *Service) WritePoisonStream(w http.ResponseWriter, flusher http.Flusher, done <-chan struct{}) {
	for {
		delay := randIntRange(2, 15)
		select {
		case <-done:
			return
		case <-time.After(time.Duration(delay) * time.Second):
			garbage := make([]byte, randIntRange(32, 512))
			_, _ = io.ReadFull(rand.Reader, garbage)
			data := hex.EncodeToString(garbage)
			_, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			if err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func randIntRange(min, max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	return int(n.Int64()) + min
}
