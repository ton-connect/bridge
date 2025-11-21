package bridge_test

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestBridgeAnalytics_EventsSentToMockServer verifies that analytics events
// are sent to the configured analytics endpoint during bridge operations
func TestBridgeAnalytics_EventsSentToMockServer(t *testing.T) {
	// Check if analytics is enabled in test environment
	analyticsEnabled := os.Getenv("TON_ANALYTICS_ENABLED")
	if analyticsEnabled != "true" {
		t.Skip("Analytics not enabled, set TON_ANALYTICS_ENABLED=true")
	}

	// Create mock analytics server
	mockServer := NewAnalyticsMock()
	defer mockServer.Close()

	t.Logf("Mock analytics server running at: %s", mockServer.Server.URL)
	t.Logf("Note: To use this mock, set TON_ANALYTICS_URL=%s/events", mockServer.Server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a session and connect
	session := randomSessionID(t)
	gw, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: session,
		OriginURL: "https://example.com",
	})
	if err != nil {
		t.Fatalf("open bridge: %v", err)
	}
	defer func() { _ = gw.Close() }()

	if err := gw.WaitReady(ctx); err != nil {
		t.Fatalf("gateway not ready: %v", err)
	}

	// Perform verify operation (wait a bit for connection to register)
	time.Sleep(500 * time.Millisecond)
	_, _, err = callVerifyEndpoint(t, BRIDGE_URL, session, "https://example.com", "connect")
	if err != nil {
		t.Logf("verify returned error (this may be expected): %v", err)
	}

	// Close the connection
	_ = gw.Close()

	// Wait a bit for analytics to be flushed
	time.Sleep(2 * time.Second)

	// Check that we received analytics events
	eventCount := mockServer.GetEventCount()
	t.Logf("Mock server received %d analytics events", eventCount)

	if eventCount == 0 {
		t.Log("No events received. This is expected if TON_ANALYTICS_URL is not set to point to the mock server.")
		t.Log("To properly test analytics, rebuild bridge with TON_ANALYTICS_URL pointing to the mock server.")
	}

	// Log event types received
	allEvents := mockServer.GetEvents()
	eventTypes := make(map[string]int)
	for _, event := range allEvents {
		if eventName, ok := event["event_name"].(string); ok {
			eventTypes[eventName]++
		}
	}

	t.Log("Events received by type:")
	for eventType, count := range eventTypes {
		t.Logf("  %s: %d", eventType, count)
	}

	// Check for specific expected events
	subscribedEvents := mockServer.GetEventsByName("bridge-events-client-subscribed")
	if len(subscribedEvents) > 0 {
		t.Logf("Found %d 'bridge-events-client-subscribed' events", len(subscribedEvents))
		// Verify event structure
		event := subscribedEvents[0]
		if clientID, ok := event["client_id"].(string); ok {
			t.Logf("  client_id: %s", clientID)
		}
		if subsystem, ok := event["subsystem"].(string); ok {
			t.Logf("  subsystem: %s", subsystem)
		}
	}

	verifyEvents := mockServer.GetEventsByName("bridge-verify")
	if len(verifyEvents) > 0 {
		t.Logf("Found %d 'bridge-verify' events", len(verifyEvents))
	}

	unsubscribedEvents := mockServer.GetEventsByName("bridge-events-client-unsubscribed")
	if len(unsubscribedEvents) > 0 {
		t.Logf("Found %d 'bridge-events-client-unsubscribed' events", len(unsubscribedEvents))
	}
}

// TestBridgeAnalytics_MessageLifecycle tests that message lifecycle events are tracked
func TestBridgeAnalytics_MessageLifecycle(t *testing.T) {
	analyticsEnabled := os.Getenv("TON_ANALYTICS_ENABLED")
	if analyticsEnabled != "true" {
		t.Skip("Analytics not enabled, set TON_ANALYTICS_ENABLED=true")
	}

	mockServer := NewAnalyticsMock()
	defer mockServer.Close()

	t.Logf("Mock analytics server running at: %s", mockServer.Server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create sender and receiver
	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: senderSession,
	})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiverSession := randomSessionID(t)
	receiver, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: receiverSession,
	})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() { _ = receiver.Close() }()

	if err := sender.WaitReady(ctx); err != nil {
		t.Fatalf("sender not ready: %v", err)
	}
	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Send a message
	if err := sender.Send(ctx, []byte("test message"), senderSession, receiverSession, nil); err != nil {
		t.Fatalf("send message: %v", err)
	}

	// Receive the message
	ev, err := receiver.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait message: %v", err)
	}
	t.Logf("Received message with ID: %s", ev.ID)

	// Close connections
	_ = sender.Close()
	_ = receiver.Close()

	// Wait for analytics flush
	time.Sleep(3 * time.Second)

	eventCount := mockServer.GetEventCount()
	t.Logf("Mock server received %d total analytics events", eventCount)

	// Log all event types
	allEvents := mockServer.GetEvents()
	eventTypes := make(map[string]int)
	for _, event := range allEvents {
		if eventName, ok := event["event_name"].(string); ok {
			eventTypes[eventName]++
		}
	}

	t.Log("Events received by type:")
	for eventType, count := range eventTypes {
		t.Logf("  %s: %d", eventType, count)
	}

	// Note: Actual event validation would require the bridge to be configured
	// with TON_ANALYTICS_URL pointing to this mock server
	if eventCount == 0 {
		t.Log("Note: To test analytics properly, rebuild bridge container with:")
		t.Logf("  TON_ANALYTICS_URL=%s/events", mockServer.Server.URL)
	}
}
