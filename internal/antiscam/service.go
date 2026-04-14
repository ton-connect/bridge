package antiscam

import (
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
// When true the caller should reject the request and close the connection.
func (s *Service) CheckSSE(origin, traceId string) bool {
	if !s.checker.IsBlocked(origin) {
		return false
	}
	BlockedConnectionsMetric.Inc()
	logrus.WithFields(logrus.Fields{"origin": origin, "trace_id": traceId}).
		Info("SSE connection rejected by antiscam filter")
	return true
}
