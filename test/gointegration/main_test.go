package bridge_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	setupSharedWebhookMock()
	code := m.Run()
	if sharedWebhookMock != nil {
		sharedWebhookMock.Close()
	}
	os.Exit(code)
}
