package bridge_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBridge_MultipleListenersSameID tests that multiple clients can listen to the same ID
// and all receive messages, and that disconnecting one client doesn't affect others.
func TestBridge_MultipleListenersSameID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*testSSETimeout)
	defer cancel()

	// Create a single recipient ID that all clients will listen to
	recipientID := randomSessionID(t)
	const numClients = 5

	// Step 1: Connect 5 clients to listen to the same ID
	receivers := make([]*BridgeGateway, 0, numClients)
	allReceivers := make([]*BridgeGateway, 0, numClients) // Keep all receivers for cleanup
	for i := 0; i < numClients; i++ {
		receiver, err := OpenBridge(ctx, OpenOpts{
			BridgeURL: BRIDGE_URL,
			SessionID: recipientID,
		})
		if err != nil {
			t.Fatalf("failed to open receiver %d: %v", i+1, err)
		}
		receivers = append(receivers, receiver)
		allReceivers = append(allReceivers, receiver)

		// Wait for each receiver to be ready
		if err := receiver.WaitReady(ctx); err != nil {
			t.Fatalf("receiver %d not ready: %v", i+1, err)
		}
	}

	// Defer cleanup for all receivers
	defer func() {
		for i, receiver := range allReceivers {
			if receiver != nil {
				if err := receiver.Close(); err != nil {
					t.Logf("error closing receiver %d: %v", i+1, err)
				}
			}
		}
	}()

	// Create a sender
	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: senderSession,
	})
	if err != nil {
		t.Fatalf("failed to open sender: %v", err)
	}
	defer func() {
		if err := sender.Close(); err != nil {
			t.Logf("error closing sender: %v", err)
		}
	}()

	if !sender.IsReady() {
		t.Fatal("sender not ready")
	}

	// Give all connections time to establish
	time.Sleep(200 * time.Millisecond)

	// Step 2: Send 1 message and verify 5 deliveries
	message1 := "test-message-1"
	if err := sender.Send(ctx, []byte(message1), senderSession, recipientID, nil); err != nil {
		t.Fatalf("failed to send first message: %v", err)
	}

	// Collect messages from all receivers concurrently
	var wg sync.WaitGroup
	deliveries1 := make([]bool, numClients)
	deliveryMutex := sync.Mutex{}

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ev, err := receivers[idx].WaitMessage(ctx)
			if err != nil {
				t.Errorf("receiver %d failed to receive message: %v", idx+1, err)
				return
			}

			var bm bridgeMessage
			if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
				t.Errorf("receiver %d failed to decode message: %v", idx+1, err)
				return
			}

			raw, err := base64.StdEncoding.DecodeString(bm.Message)
			if err != nil {
				t.Errorf("receiver %d failed to decode base64: %v", idx+1, err)
				return
			}

			if string(raw) == message1 && bm.From == senderSession {
				deliveryMutex.Lock()
				deliveries1[idx] = true
				deliveryMutex.Unlock()
			} else {
				t.Errorf("receiver %d received unexpected message: from=%s, msg=%s", idx+1, bm.From, string(raw))
			}
		}(i)
	}

	// Wait for all deliveries with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All messages received
	case <-time.After(testSSETimeout):
		t.Fatal("timeout waiting for first message deliveries")
	}

	// Verify all 5 clients received the message
	deliveryMutex.Lock()
	deliveredCount := 0
	for i, delivered := range deliveries1 {
		if delivered {
			deliveredCount++
		} else {
			t.Errorf("receiver %d did not receive first message", i+1)
		}
	}
	deliveryMutex.Unlock()

	if deliveredCount != numClients {
		t.Fatalf("expected %d deliveries, got %d", numClients, deliveredCount)
	}
	t.Logf("✓ All %d clients received the first message", numClients)

	// Step 3: Disconnect one client (the first one)
	t.Logf("Disconnecting receiver 1...")
	if err := receivers[0].Close(); err != nil {
		t.Fatalf("failed to close receiver 1: %v", err)
	}
	allReceivers[0] = nil     // Mark as closed so defer won't try to close it again
	receivers = receivers[1:] // Remove from slice

	// Give time for disconnection to propagate
	time.Sleep(200 * time.Millisecond)

	// Step 4: Send 1 message
	message2 := "test-message-2"
	if err := sender.Send(ctx, []byte(message2), senderSession, recipientID, nil); err != nil {
		t.Fatalf("failed to send second message: %v", err)
	}

	// Step 5: Verify 4 deliveries (remaining clients)
	deliveries2 := make([]bool, len(receivers))
	var wg2 sync.WaitGroup

	for i := 0; i < len(receivers); i++ {
		wg2.Add(1)
		go func(idx int) {
			defer wg2.Done()
			ev, err := receivers[idx].WaitMessage(ctx)
			if err != nil {
				t.Errorf("receiver %d failed to receive second message: %v", idx+2, err)
				return
			}

			var bm bridgeMessage
			if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
				t.Errorf("receiver %d failed to decode second message: %v", idx+2, err)
				return
			}

			raw, err := base64.StdEncoding.DecodeString(bm.Message)
			if err != nil {
				t.Errorf("receiver %d failed to decode base64: %v", idx+2, err)
				return
			}

			if string(raw) == message2 && bm.From == senderSession {
				deliveryMutex.Lock()
				deliveries2[idx] = true
				deliveryMutex.Unlock()
			} else {
				t.Errorf("receiver %d received unexpected second message: from=%s, msg=%s", idx+2, bm.From, string(raw))
			}
		}(i)
	}

	// Wait for all deliveries with timeout
	done2 := make(chan struct{})
	go func() {
		wg2.Wait()
		close(done2)
	}()

	select {
	case <-done2:
		// All messages received
	case <-time.After(testSSETimeout):
		t.Fatal("timeout waiting for second message deliveries")
	}

	// Verify 4 clients received the message
	deliveryMutex.Lock()
	deliveredCount2 := 0
	for i, delivered := range deliveries2 {
		if delivered {
			deliveredCount2++
		} else {
			t.Errorf("receiver %d did not receive second message", i+2)
		}
	}
	deliveryMutex.Unlock()

	expectedDeliveries2 := numClients - 1
	if deliveredCount2 != expectedDeliveries2 {
		t.Fatalf("expected %d deliveries after disconnection, got %d", expectedDeliveries2, deliveredCount2)
	}
	t.Logf("✓ All %d remaining clients received the second message", expectedDeliveries2)
}

// TestBridge_SingleListenerMultipleIDs tests that a single user can listen to 100 IDs
// and receive messages sent to each ID.
func TestBridge_SingleListenerMultipleIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*testSSETimeout)
	defer cancel()

	const numIDs = 100

	// Step 1: Create 100 recipient IDs
	recipientIDs := make([]string, 0, numIDs)
	for i := 0; i < numIDs; i++ {
		recipientIDs = append(recipientIDs, randomSessionID(t))
	}

	// Step 2: Connect one user to listen to all 100 IDs (comma-separated)
	multiClientParam := strings.Join(recipientIDs, ",")
	receiver, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: multiClientParam,
	})
	if err != nil {
		t.Fatalf("failed to open multi-ID receiver: %v", err)
	}
	defer func() {
		if err := receiver.Close(); err != nil {
			t.Logf("error closing receiver: %v", err)
		}
	}()

	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Give connection time to establish
	time.Sleep(200 * time.Millisecond)

	// Step 3: Create a single sender
	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL,
		SessionID: senderSession,
	})
	if err != nil {
		t.Fatalf("failed to open sender: %v", err)
	}
	defer func() {
		if err := sender.Close(); err != nil {
			t.Logf("error closing sender: %v", err)
		}
	}()

	if !sender.IsReady() {
		t.Fatal("sender not ready")
	}

	// Step 4: Send 1 message to each of the 100 IDs
	// Track which messages we sent (recipientID -> message payload)
	sentMessages := make(map[string]string, numIDs)       // recipientID -> payload
	payloadToRecipient := make(map[string]string, numIDs) // payload -> recipientID
	for i, recipientID := range recipientIDs {
		payload := fmt.Sprintf("message-to-id-%d", i+1)
		sentMessages[recipientID] = payload
		payloadToRecipient[payload] = recipientID

		if err := sender.Send(ctx, []byte(payload), senderSession, recipientID, nil); err != nil {
			t.Fatalf("failed to send message to recipient %d (ID: %s): %v", i+1, recipientID[:16], err)
		}
	}

	t.Logf("✓ Sent %d messages to %d different IDs", numIDs, numIDs)

	// Step 5: Collect all received messages
	// Track received payloads (payload -> recipientID)
	receivedPayloads := make(map[string]string, numIDs) // payload -> recipientID
	receivedMutex := sync.Mutex{}

	// Use a goroutine to collect messages with timeout
	done := make(chan struct{})
	go func() {
		for len(receivedPayloads) < numIDs {
			ev, err := receiver.WaitMessage(ctx)
			if err != nil {
				t.Errorf("failed to receive message: %v", err)
				return
			}

			var bm bridgeMessage
			if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
				t.Errorf("failed to decode message: %v", err)
				continue
			}

			raw, err := base64.StdEncoding.DecodeString(bm.Message)
			if err != nil {
				t.Errorf("failed to decode base64: %v", err)
				continue
			}

			payload := string(raw)

			// Verify sender and find matching recipient ID
			if bm.From != senderSession {
				t.Errorf("received message from unexpected sender: %s (expected %s)", bm.From, senderSession)
				continue
			}

			receivedMutex.Lock()
			// Check if this payload matches one we sent
			if recipientID, expected := payloadToRecipient[payload]; expected {
				// Check if we already received this payload
				if _, exists := receivedPayloads[payload]; !exists {
					receivedPayloads[payload] = recipientID
				}
			} else {
				t.Errorf("received unexpected payload: %q", payload)
			}
			receivedMutex.Unlock()
		}
		close(done)
	}()

	// Wait for all messages with timeout
	select {
	case <-done:
		// All messages received
	case <-time.After(5 * testSSETimeout):
		receivedMutex.Lock()
		receivedCount := len(receivedPayloads)
		receivedMutex.Unlock()
		t.Fatalf("timeout waiting for messages: received %d out of %d", receivedCount, numIDs)
	}

	// Step 6: Verify all messages were received
	receivedMutex.Lock()
	defer receivedMutex.Unlock()

	if len(receivedPayloads) != numIDs {
		t.Fatalf("expected %d messages, received %d", numIDs, len(receivedPayloads))
	}

	// Verify each recipient ID received its message
	missingIDs := make([]string, 0)
	for i, recipientID := range recipientIDs {
		expectedPayload := sentMessages[recipientID]
		receivedRecipientID, ok := receivedPayloads[expectedPayload]
		if !ok {
			missingIDs = append(missingIDs, fmt.Sprintf("ID-%d (%s...)", i+1, recipientID[:16]))
		} else if receivedRecipientID != recipientID {
			t.Errorf("recipient %d: payload %q was received for wrong ID (expected %s, got %s)", i+1, expectedPayload, recipientID[:16], receivedRecipientID[:16])
		}
	}

	if len(missingIDs) > 0 {
		t.Fatalf("missing messages for %d recipients: %v", len(missingIDs), missingIDs[:min(10, len(missingIDs))])
	}

	t.Logf("✓ All %d messages received successfully", numIDs)
}
