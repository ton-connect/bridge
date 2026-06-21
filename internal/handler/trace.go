package handler

import (
	"log/slog"

	"github.com/google/uuid"
)

func ParseOrGenerateTraceID(traceIdParam string, ok bool) string {
	logger := slog.With("prefix", "CreateSession")
	traceId := "unknown"
	if ok {
		uuids, err := uuid.Parse(traceIdParam)
		if err != nil {
			logger.Warn("generating a new trace_id", "err", err, "invalid_trace_id", traceIdParam)
		} else {
			traceId = uuids.String()
		}
	}
	if traceId == "unknown" {
		uuids, err := uuid.NewV7()
		if err != nil {
			logger.Error("failed to generate trace_id", "err", err)
		} else {
			traceId = uuids.String()
		}
	}
	return traceId
}
