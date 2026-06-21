// Package obs is the bridge observability contract: a JSON slog on stdout with
// constant service + git_sha attributes, installed as the slog default in each
// binary's main. During the v1→v3 logging transition the v3 path logs through
// this; v1-specific code still uses logrus (configured in internal/config).
package obs

import (
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
)

// Setup builds the root JSON logger on w, filtered at level (unknown → info),
// tagged with constant service + git_sha attributes.
func Setup(w io.Writer, level, service string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug", "trace":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error", "fatal", "panic":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l})
	return slog.New(h).With(slog.String("service", service), slog.String("git_sha", gitSHAShort()))
}

// gitSHAShort returns the short VCS revision the binary was built from (build-info
// stamp), falling back to GIT_SHA_SHORT when there is no stamp (e.g. `go run`).
func gitSHAShort() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return s.Value[:min(7, len(s.Value))]
			}
		}
	}
	return os.Getenv("GIT_SHA_SHORT")
}
