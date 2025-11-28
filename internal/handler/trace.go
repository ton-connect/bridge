package handler

import (
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func ParseOrGenerateTraceID(traceIdParam string, ok bool) string {
	log := logrus.WithField("prefix", "CreateSession")
	traceId := "unknown"
	if ok {
		uuids, err := uuid.Parse(traceIdParam)
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
	return traceId
}
