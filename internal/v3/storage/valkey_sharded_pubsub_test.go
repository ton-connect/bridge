package storagev3

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tonkeeper/bridge/internal/models"
)

// TestShardedPubSubManager_MultipleShards tests that ShardedPubSubManager
// correctly handles subscriptions to channels on different shards
func TestShardedPubSubManager_MultipleShards(t *testing.T) {
	// This test demonstrates the solution to the go-redis limitation
	// where SSUBSCRIBE only works with channels on the same shard.

	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Note: This test requires a running Redis/Valkey cluster
	// Run with: docker-compose -f docker/docker-compose.cluster-valkey.yml up

	t.Skip("This is a reference test showing the expected behavior. Run manually with a Redis cluster.")

	// Create manager
	manager, err := NewShardedPubSubManager(
		[]string{"localhost:7001", "localhost:7002", "localhost:7003"},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	ctx := context.Background()

	// These channels will likely be on different shards
	// The key insight: valkey-go handles this correctly, go-redis does not
	channels := []string{
		"client:test_user_1",
		"client:test_user_2",
		"client:test_user_3",
		"client:test_user_4",
		"client:test_user_5",
	}

	// Create message channels for each subscription
	messageChannels := make([]chan models.SseMessage, len(channels))
	for i := range messageChannels {
		messageChannels[i] = make(chan models.SseMessage, 10)
	}

	// Subscribe to all channels
	for i, channel := range channels {
		if err := manager.Subscribe(ctx, channel, messageChannels[i]); err != nil {
			t.Fatalf("Failed to subscribe to %s: %v", channel, err)
		}
	}

	// Give subscriptions time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish messages to all channels
	for i, channel := range channels {
		msg := models.SseMessage{
			To:      channel[7:], // Remove "client:" prefix
			Message: []byte("test message"),
			EventId: int64(i),
		}

		msgBytes, _ := json.Marshal(msg)
		if err := manager.Publish(ctx, channel, msgBytes); err != nil {
			t.Fatalf("Failed to publish to %s: %v", channel, err)
		}
	}

	// Verify all messages are received
	// This is the key test: with go-redis SSUBSCRIBE, only messages from
	// the first shard would be received. With valkey-go, all are received.
	receivedCount := 0
	timeout := time.After(2 * time.Second)

	for i := 0; i < len(channels); i++ {
		select {
		case msg := <-messageChannels[i]:
			t.Logf("Received message on channel %s: %+v", channels[i], msg)
			receivedCount++
		case <-timeout:
			t.Logf("Timeout waiting for message on channel %s", channels[i])
		}
	}

	if receivedCount != len(channels) {
		t.Errorf("Expected to receive %d messages, got %d", len(channels), receivedCount)
		t.Logf("This indicates messages from some shards were not received")
	} else {
		t.Logf("SUCCESS: All %d messages received across different shards", receivedCount)
	}

	// Cleanup
	for i, channel := range channels {
		if err := manager.Unsubscribe(channel, messageChannels[i]); err != nil {
			t.Logf("Warning: Failed to unsubscribe from %s: %v", channel, err)
		}
	}
}

// TestShardedPubSubManager_ConcurrentSubscribers tests that multiple
// subscribers can listen to the same channel
func TestShardedPubSubManager_ConcurrentSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Skip("This is a reference test. Run manually with a Redis cluster.")

	manager, err := NewShardedPubSubManager(
		[]string{"localhost:7001"},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	ctx := context.Background()
	channel := "client:test_multi_subscriber"

	// Create multiple subscribers for the same channel
	subscriber1 := make(chan models.SseMessage, 10)
	subscriber2 := make(chan models.SseMessage, 10)
	subscriber3 := make(chan models.SseMessage, 10)

	// Subscribe all
	if err := manager.Subscribe(ctx, channel, subscriber1); err != nil {
		t.Fatalf("Failed to subscribe 1: %v", err)
	}
	if err := manager.Subscribe(ctx, channel, subscriber2); err != nil {
		t.Fatalf("Failed to subscribe 2: %v", err)
	}
	if err := manager.Subscribe(ctx, channel, subscriber3); err != nil {
		t.Fatalf("Failed to subscribe 3: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish one message
	msg := models.SseMessage{
		To:      "test_multi_subscriber",
		Message: []byte("broadcast"),
		EventId: 1,
	}
	msgBytes, _ := json.Marshal(msg)
	if err := manager.Publish(ctx, channel, msgBytes); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// All three subscribers should receive it
	timeout := time.After(1 * time.Second)
	received := 0

	for i := 0; i < 3; i++ {
		select {
		case <-subscriber1:
			received++
			t.Logf("Subscriber 1 received message")
		case <-subscriber2:
			received++
			t.Logf("Subscriber 2 received message")
		case <-subscriber3:
			received++
			t.Logf("Subscriber 3 received message")
		case <-timeout:
			t.Fatalf("Timeout waiting for messages")
		}
	}

	if received != 3 {
		t.Errorf("Expected 3 receives, got %d", received)
	}
}
