// Package obs is the bridge observability contract: a JSON slog on stdout with
// constant service + git_sha attributes, installed as the slog default in each
// binary's main. During the v1→v3 logging transition the v3 path logs through
// this; v1-specific code still uses logrus (configured in internal/config).
package obs

import (
	"io"
	"log/slog"
	"strings"

	"github.com/ton-connect/bridge/internal"
)

// Setup builds the root JSON logger on w, filtered at level, tagged with constant
// service + git_sha attributes. An unrecognized level falls back to info and the
// returned logger emits a warning, so a misconfigured LOG_LEVEL is not silent.
func Setup(w io.Writer, level, service string) *slog.Logger {
	l, ok := parseLevel(level)
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l})
	logger := slog.New(h).With(slog.String("service", service), slog.String("git_sha", gitSHAShort()))
	if !ok {
		logger.Warn("unrecognized LOG_LEVEL, using info", "value", level)
	}
	return logger
}

// parseLevel maps a textual level to a slog.Level. ok is false when level is not a
// recognized name, in which case the caller falls back to info and warns.
func parseLevel(level string) (slog.Level, bool) {
	switch strings.ToLower(level) {
	case "debug", "trace":
		return slog.LevelDebug, true
	case "", "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error", "fatal", "panic":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// gitSHAShort returns the git revision the binary was built from, for the git_sha
// log attribute. It uses internal.GitRevision, injected at build time via -X ldflags
// (see Makefile); in un-injected builds (go run, tests) it is the "devel" default.
func gitSHAShort() string {
	return internal.GitRevision
}
