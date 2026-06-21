package obs_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ton-connect/bridge/internal/obs"
)

func TestSetupEmitsJSONWithServiceAndGitSHA(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.Setup(&buf, "info", "bridge")
	logger.Info("hello", "k", "v")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "INFO", rec["level"])
	require.Equal(t, "hello", rec["msg"])
	require.Equal(t, "bridge", rec["service"])
	require.Equal(t, "v", rec["k"])
	_, ok := rec["git_sha"]
	require.True(t, ok)
}

func TestSetupUnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.Setup(&buf, "bogus", "bridge")
	logger.Debug("suppressed")
	require.Empty(t, buf.String())
	logger.Info("kept")
	require.Contains(t, buf.String(), `"msg":"kept"`)
}
