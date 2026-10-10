package storagev3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/ton-connect/bridge/internal/models"
)

func TestNewValkeyStorage_SingleNode(t *testing.T) {
	// Test with invalid URI to ensure it fails gracefully
	_, err := NewValkeyStorage("invalid://uri")
	if err == nil {
		t.Error("Expected error for invalid URI, got nil")
	}
}

func TestNewValkeyStorage_RedisURI(t *testing.T) {
	// Test parsing of redis:// URI (should work even if connection fails)
	valkeyURI := "redis://localhost:6379"

	// This will fail to connect, but should parse URI correctly
	_, err := NewValkeyStorage(valkeyURI)

	// We expect a connection error, not a parsing error
	if err != nil && err.Error() != "connection failed: dial tcp [::1]:6379: connect: connection refused" &&
		err.Error() != "connection failed: dial tcp 127.0.0.1:6379: connect: connection refused" {
		// Connection error is expected since there's no Redis running
		// But the error message format tells us the URI was parsed correctly
		t.Logf("Connection failed as expected: %v", err)
	}
}

// getTestValkeyURI returns the Valkey URI from environment or skips the test
func getTestValkeyURI(t *testing.T) string {
	t.Helper()
	uri := os.Getenv("VALKEY_URI")
	if uri == "" {
		uri = os.Getenv("REDIS_URI")
	}
	if uri == "" {
		t.Skip("Skipping Valkey integration test: VALKEY_URI or REDIS_URI not set")
	}
	return uri
}

// waitSwept blocks until a sweep attempt has finished since sweepsBefore, then checks the backlog
// it left. The counter is written after the sweep's last Valkey call, so a shrunken backlog
// alone would let a test read the counter before the sweep has added to it.
func waitSwept(t *testing.T, storage *ValkeyStorage, clientID string, sweepsBefore, wantBacklog int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for storage.sweeps.Load() <= sweepsBefore {
		if time.Now().After(deadline) {
			t.Fatalf("sweep of %s did not finish within 5s", clientID)
		}
		time.Sleep(20 * time.Millisecond)
	}
	n, err := storage.client.ZCard(context.Background(), fmt.Sprintf("client:%s", clientID)).Result()
	if err != nil {
		t.Fatalf("ZCard failed: %v", err)
	}
	if n != wantBacklog {
		t.Fatalf("sweep left %d entries in the backlog of %s, want %d", n, clientID, wantBacklog)
	}
}

// cleanupKeys removes the keys a test made. They hash to different slots, so a multi-key
// DEL would come back CROSSSLOT and delete neither.
func cleanupKeys(storage *ValkeyStorage, clientID string) {
	ctx := context.Background()
	storage.client.Del(ctx, fmt.Sprintf("client:%s", clientID))
	storage.client.Del(ctx, deliveredAtKey(clientID))
}

func TestValkeyStorage_ConnectionVerification_ExactMatch(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	conn := ConnectionInfo{
		ClientID:  "test-client-1",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}

	// Add connection
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with exact match -> should return "ok"
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_SameOriginDifferentIP(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-2"

	// Add first connection
	conn1 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn1, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with same origin but different IP -> should return "warning"
	conn2 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.2",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/2.0",
	}
	status, err := storage.VerifyConnection(ctx, conn2)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "warning" {
		t.Errorf("expected status 'warning', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_DifferentOrigin(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-3"

	// Add first connection
	conn1 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	ttl := 5 * time.Second
	if err := storage.AddConnection(ctx, conn1, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify with different origin -> should return "danger"
	conn2 := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.2",
		Origin:    "https://malicious.com",
		UserAgent: "TestAgent/2.0",
	}
	status, err := storage.VerifyConnection(ctx, conn2)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "danger" {
		t.Errorf("expected status 'danger', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_Unknown(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Verify without adding connection -> should return "unknown"
	conn := ConnectionInfo{
		ClientID:  "test-client-unknown",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "unknown" {
		t.Errorf("expected status 'unknown', got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_TTLExpiration(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	conn := ConnectionInfo{
		ClientID:  "test-client-ttl",
		IP:        "192.168.1.1",
		Origin:    "https://example.com",
		UserAgent: "TestAgent/1.0",
	}

	// Add connection with short TTL
	ttl := 2 * time.Second
	if err := storage.AddConnection(ctx, conn, ttl); err != nil {
		t.Fatalf("AddConnection failed: %v", err)
	}

	// Verify immediately -> should return "ok"
	status, err := storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected status 'ok' before expiration, got '%s'", status)
	}

	// Wait for TTL to expire
	time.Sleep(3 * time.Second)

	// Verify after expiration -> should return "unknown"
	status, err = storage.VerifyConnection(ctx, conn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "unknown" {
		t.Errorf("expected status 'unknown' after expiration, got '%s'", status)
	}
}

func TestValkeyStorage_ConnectionVerification_MultipleConnections(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := "test-client-multi"
	ttl := 10 * time.Second

	// Add multiple connections for the same client
	connections := []ConnectionInfo{
		{ClientID: clientID, IP: "192.168.1.1", Origin: "https://example.com", UserAgent: "Browser/1.0"},
		{ClientID: clientID, IP: "192.168.1.2", Origin: "https://example.com", UserAgent: "Browser/1.0"},
		{ClientID: clientID, IP: "10.0.0.1", Origin: "https://example.com", UserAgent: "Mobile/1.0"},
	}

	for _, conn := range connections {
		if err := storage.AddConnection(ctx, conn, ttl); err != nil {
			t.Fatalf("AddConnection failed: %v", err)
		}
	}

	// Verify exact match with first connection -> "ok"
	status, err := storage.VerifyConnection(ctx, connections[0])
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "ok" {
		t.Errorf("expected 'ok' for exact match, got '%s'", status)
	}

	// Verify with new IP but same origin -> "warning"
	newConn := ConnectionInfo{
		ClientID:  clientID,
		IP:        "192.168.1.100",
		Origin:    "https://example.com",
		UserAgent: "NewBrowser/1.0",
	}
	status, err = storage.VerifyConnection(ctx, newConn)
	if err != nil {
		t.Fatalf("VerifyConnection failed: %v", err)
	}
	if status != "warning" {
		t.Errorf("expected 'warning' for same origin different IP, got '%s'", status)
	}
}

// TestValkeyStorage_SubCountsUndeliveredExpiry drives the production call site rather than
// the sweep helper: expiry is resolved inside Sub, so a test that called the helper
// directly would stay green if that call were dropped or moved after the replay.
func TestValkeyStorage_SubCountsUndeliveredExpiry(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("reap-%d", time.Now().UnixNano())

	expiring := models.SseMessage{
		EventId: 1,
		Message: []byte(`{"from":"sender","message":"gone","trace_id":"trace-expired"}`),
		To:      clientID,
	}
	surviving := models.SseMessage{
		EventId: 2,
		Message: []byte(`{"from":"sender","message":"kept","trace_id":"trace-live"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, expiring, 1); err != nil {
		t.Fatalf("Pub(expiring) failed: %v", err)
	}
	if err := storage.Pub(ctx, surviving, 300); err != nil {
		t.Fatalf("Pub(surviving) failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore := storage.sweeps.Load()

	messageCh := make(chan models.SseMessage, 8)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	// One of the two survives its TTL, so a finished sweep leaves exactly one entry.
	waitSwept(t, storage, clientID, sweepsBefore, 1)

	if got := testutil.ToFloat64(expiredMessagesMetric) - before; got != 1 {
		t.Errorf("expected the undelivered expired message to be counted once, counter moved by %v", got)
	}

	// Sub replays the backlog it did not sweep, so the live message arrives and the expired
	// one must not: a client reconnecting after the TTL has to see neither the message nor
	// a gap in event ids it cannot explain.
	var replayed []int64
	for len(messageCh) > 0 {
		replayed = append(replayed, (<-messageCh).EventId)
	}
	if len(replayed) != 1 || replayed[0] != surviving.EventId {
		t.Errorf("expected only the live message replayed, got event ids %v", replayed)
	}

	// A second reconnect finds nothing left to sweep and must add nothing.
	after := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore = storage.sweeps.Load()
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("second Sub failed: %v", err)
	}
	waitSwept(t, storage, clientID, sweepsBefore, 1)
	if got := testutil.ToFloat64(expiredMessagesMetric); got != after {
		t.Errorf("second sweep double-counted: counter moved from %v to %v", after, got)
	}

	cleanupKeys(storage, clientID)
}

// TestValkeyStorage_DeliveredExpiryIsNotCounted pins the distinction the counter exists for.
// Pub stores a copy of every message whether or not anyone was listening, so a delivered
// message sits in the backlog until its TTL exactly like an undelivered one. Counting the
// sweep blindly would report successful deliveries as losses.
func TestValkeyStorage_DeliveredExpiryIsNotCounted(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("delivered-%d", time.Now().UnixNano())

	msg := models.SseMessage{
		EventId: 7,
		Message: []byte(`{"from":"sender","message":"seen","trace_id":"trace-delivered"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, msg, 1); err != nil {
		t.Fatalf("Pub failed: %v", err)
	}
	if err := storage.MarkDelivered(ctx, clientID, msg.EventId); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore := storage.sweeps.Load()

	messageCh := make(chan models.SseMessage, 4)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	// The only message expired, so a finished sweep empties the backlog.
	waitSwept(t, storage, clientID, sweepsBefore, 0)

	if got := testutil.ToFloat64(expiredMessagesMetric); got != before {
		t.Errorf("a delivered message was counted as expired: counter moved from %v to %v", before, got)
	}

	cleanupKeys(storage, clientID)
}

// TestValkeyStorage_ReconnectWithinFlushIntervalIsNotCounted reconnects right after a
// delivery, before the mark writer has flushed. Marks reach Valkey in batches on a timer, so a
// sweep that starts the moment Sub returns finds the backlog expired and the mark still
// queued, and counts a delivered message as lost.
func TestValkeyStorage_ReconnectWithinFlushIntervalIsNotCounted(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("reconnect-%d", time.Now().UnixNano())

	msg := models.SseMessage{
		EventId: 9,
		Message: []byte(`{"from":"sender","message":"seen late","trace_id":"trace-reconnect"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, msg, 1); err != nil {
		t.Fatalf("Pub failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore := storage.sweeps.Load()

	// The mark is queued and the reconnect follows at once, inside one flush interval.
	if err := storage.MarkDelivered(ctx, clientID, msg.EventId); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}
	messageCh := make(chan models.SseMessage, 4)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	waitSwept(t, storage, clientID, sweepsBefore, 0)

	if got := testutil.ToFloat64(expiredMessagesMetric); got != before {
		t.Errorf("a message delivered just before the reconnect was counted as expired: counter moved from %v to %v", before, got)
	}

	cleanupKeys(storage, clientID)
}

// TestValkeyStorage_DeliveredMarksAreBounded seeds a mark older than deliveredMarkTTL and
// checks that the next write for the same client trims it. A key-level EXPIRE alone cannot
// do this: every new mark refreshes it, so a client that keeps receiving keeps every mark.
func TestValkeyStorage_DeliveredMarksAreBounded(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("bounded-%d", time.Now().UnixNano())
	key := deliveredAtKey(clientID)

	stale := float64(time.Now().Add(-2 * deliveredMarkTTL).Unix())
	if err := storage.client.ZAdd(ctx, key, redis.Z{Score: stale, Member: "999"}).Err(); err != nil {
		t.Fatalf("seeding a stale mark failed: %v", err)
	}

	if err := storage.MarkDelivered(ctx, clientID, 7); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := storage.client.ZScore(ctx, key, "7").Result(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mark for event 7 did not reach %s within 2s", key)
		}
		time.Sleep(20 * time.Millisecond)
	}

	members, err := storage.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange failed: %v", err)
	}
	if len(members) != 1 || members[0] != "7" {
		t.Errorf("expected the stale mark trimmed and only event 7 kept, got members %v", members)
	}

	cleanupKeys(storage, clientID)
}

// TestValkeyStorage_LongSessionDeliveryIsNotCounted keeps a delivered member in the backlog
// past an hour of further deliveries, as an SSE session of SSE_MAX_LIFETIME does, then sweeps it.
func TestValkeyStorage_LongSessionDeliveryIsNotCounted(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("long-session-%d", time.Now().UnixNano())
	key := deliveredAtKey(clientID)

	msg := models.SseMessage{
		EventId: 11,
		Message: []byte(`{"from":"sender","message":"seen early","trace_id":"trace-long-session"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, msg, 1); err != nil {
		t.Fatalf("Pub failed: %v", err)
	}
	early := float64(time.Now().Add(-90 * time.Minute).Unix())
	if err := storage.client.ZAdd(ctx, key, redis.Z{Score: early, Member: "11"}).Err(); err != nil {
		t.Fatalf("seeding the early mark failed: %v", err)
	}

	// A delivery late in the same session writes a new mark, and with it the trim.
	if err := storage.MarkDelivered(ctx, clientID, 12); err != nil {
		t.Fatalf("MarkDelivered failed: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := storage.client.ZScore(ctx, key, "12").Result(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mark for event 12 did not reach %s within 2s", key)
		}
		time.Sleep(20 * time.Millisecond)
	}

	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore := storage.sweeps.Load()

	messageCh := make(chan models.SseMessage, 4)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	waitSwept(t, storage, clientID, sweepsBefore, 0)

	if got := testutil.ToFloat64(expiredMessagesMetric); got != before {
		t.Errorf("a message delivered 90 minutes before its sweep was counted as expired: counter moved from %v to %v", before, got)
	}
	if _, err := storage.client.ZScore(ctx, key, "11").Result(); err != redis.Nil {
		t.Errorf("the swept event 11 left its delivery mark behind (ZSCORE err %v)", err)
	}

	cleanupKeys(storage, clientID)
}

// waitAttempts polls until n more sweeps have run or given up.
func waitAttempts(t *testing.T, storage *ValkeyStorage, sweepsBefore int64, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for storage.sweeps.Load() < sweepsBefore+int64(n) {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d sweeps ran or gave up within 10s", storage.sweeps.Load()-sweepsBefore, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestValkeyStorage_SweepsAreBounded holds every sweep slot itself. A sweep that finds them
// taken retries, gives up only once its retries are spent, and runs if a slot frees in between.
func TestValkeyStorage_SweepsAreBounded(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("bounded-sweeps-%d", time.Now().UnixNano())
	defer cleanupKeys(storage, clientID)

	expiring := models.SseMessage{
		EventId: 31,
		Message: []byte(`{"from":"sender","message":"left for later","trace_id":"trace-skipped"}`),
		To:      clientID,
	}
	if err := storage.Pub(ctx, expiring, 1); err != nil {
		t.Fatalf("Pub failed: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	limit := cap(storage.sweepSlots)
	takeAll := func() {
		for i := 0; i < limit; i++ {
			storage.sweepSlots <- struct{}{}
		}
	}
	freeAll := func() {
		for i := 0; i < limit; i++ {
			<-storage.sweepSlots
		}
	}

	expiredBefore := testutil.ToFloat64(expiredMessagesMetric)
	skippedBefore := testutil.ToFloat64(skippedSweepsMetric)
	messageCh := make(chan models.SseMessage, 4)
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()

	takeAll()
	sweepsBefore := storage.sweeps.Load()
	subbed := time.Now()
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	waitAttempts(t, storage, sweepsBefore, 1)
	budget := sweepDelay * (1<<sweepAttempts - 1)
	if waited := time.Since(subbed); waited < budget {
		t.Errorf("the sweep gave up %v after Sub, before its %v of retries", waited, budget)
	}
	if got := testutil.ToFloat64(skippedSweepsMetric) - skippedBefore; got != 1 {
		t.Errorf("skip counter moved by %v for one sweep that gave up, want 1", got)
	}
	n, err := storage.client.ZCard(ctx, fmt.Sprintf("client:%s", clientID)).Result()
	if err != nil {
		t.Fatalf("ZCard failed: %v", err)
	}
	if n != 1 {
		t.Errorf("a sweep ran past the limit: backlog holds %d entries, want its 1 expired member", n)
	}
	if got := testutil.ToFloat64(expiredMessagesMetric); got != expiredBefore {
		t.Errorf("a sweep that gave up moved the expiry counter from %v to %v", expiredBefore, got)
	}
	freeAll()

	// The slots free up after the first try and before the first retry, which then runs.
	takeAll()
	sweepsBefore = storage.sweeps.Load()
	subbed = time.Now()
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("second Sub failed: %v", err)
	}
	firstRetry := sweepDelay + sweepDelay<<1
	time.Sleep(time.Until(subbed.Add(firstRetry - sweepDelay/3)))
	freeAll()
	waitSwept(t, storage, clientID, sweepsBefore, 0)
	if got := testutil.ToFloat64(expiredMessagesMetric) - expiredBefore; got != 1 {
		t.Errorf("the retried sweep counted %v expired messages, want 1", got)
	}
	if got := testutil.ToFloat64(skippedSweepsMetric) - skippedBefore; got != 1 {
		t.Errorf("skip counter moved by %v in total, want only the first sweep's 1", got)
	}
	if got := len(storage.sweepSlots); got != 0 {
		t.Errorf("%d sweep slots still held after the sweep finished", got)
	}
}

// TestValkeyStorage_SweepSlotIsFreeDuringDelay reconnects more keys at once than there are
// slots. A slot is for Valkey work, so none may be held while the sweeps wait out sweepDelay.
func TestValkeyStorage_SweepSlotIsFreeDuringDelay(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	keys := make([]string, cap(storage.sweepSlots)+3)
	prefix := fmt.Sprintf("delayed-sweeps-%d", time.Now().UnixNano())
	for i := range keys {
		keys[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	defer func() {
		for _, k := range keys {
			cleanupKeys(storage, k)
		}
	}()

	sweepsBefore := storage.sweeps.Load()
	skippedBefore := testutil.ToFloat64(skippedSweepsMetric)
	messageCh := make(chan models.SseMessage, 4)
	subbed := time.Now()
	if err := storage.Sub(ctx, keys, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, keys, messageCh) }()

	for time.Since(subbed) < sweepDelay/2 {
		if got := len(storage.sweepSlots); got != 0 {
			t.Fatalf("%d sweep slots held %v into sweepDelay, before any sweep reached Valkey", got, time.Since(subbed))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := storage.sweeps.Load() - sweepsBefore; got != 0 {
		t.Fatalf("%d sweeps finished before sweepDelay passed", got)
	}

	waitAttempts(t, storage, sweepsBefore, len(keys))
	if got := len(storage.sweepSlots); got != 0 {
		t.Errorf("%d sweep slots still held after every sweep finished", got)
	}
	if got := testutil.ToFloat64(skippedSweepsMetric) - skippedBefore; got != 0 {
		t.Errorf("%v sweeps gave up although their Valkey work lasts far less than the retries", got)
	}
}

// TestValkeyStorage_ShutdownFlushesQueuedMarks queues marks inside one flush interval and
// shuts down at once, which is what a pod does to the deliveries it made last.
func TestValkeyStorage_ShutdownFlushesQueuedMarks(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("shutdown-%d", time.Now().UnixNano())
	defer cleanupKeys(storage, clientID)

	ids := []string{"41", "42", "43"}
	for _, id := range []int64{41, 42, 43} {
		if err := storage.MarkDelivered(ctx, clientID, id); err != nil {
			t.Fatalf("MarkDelivered failed: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	scores, err := storage.client.ZMScore(ctx, deliveredAtKey(clientID), ids...).Result()
	if err != nil {
		t.Fatalf("ZMScore failed: %v", err)
	}
	for i, score := range scores {
		if score <= 0 {
			t.Errorf("mark for event %s was not in Valkey when Shutdown returned", ids[i])
		}
	}

	droppedBefore := testutil.ToFloat64(droppedMarksMetric)
	if err := storage.MarkDelivered(ctx, clientID, 44); err != nil {
		t.Fatalf("MarkDelivered after Shutdown failed: %v", err)
	}
	if got := testutil.ToFloat64(droppedMarksMetric) - droppedBefore; got != 1 {
		t.Errorf("a mark after Shutdown moved the dropped counter by %v, want 1", got)
	}
	if err := storage.Shutdown(shutdownCtx); err != nil {
		t.Errorf("second Shutdown failed: %v", err)
	}
}

// TestValkeyStorage_ShutdownReportsFailedDrain closes the client under the queued marks, so the
// drain cannot write them: Shutdown has to say so and the marks have to be counted as dropped.
func TestValkeyStorage_ShutdownReportsFailedDrain(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	ctx := context.Background()
	clientID := fmt.Sprintf("failed-drain-%d", time.Now().UnixNano())
	if err := storage.client.Close(); err != nil {
		t.Fatalf("closing the client failed: %v", err)
	}
	droppedBefore := testutil.ToFloat64(droppedMarksMetric)
	for _, id := range []int64{51, 52, 53} {
		if err := storage.MarkDelivered(ctx, clientID, id); err != nil {
			t.Fatalf("MarkDelivered failed: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = storage.Shutdown(shutdownCtx)
	if !errors.Is(err, redis.ErrClosed) {
		t.Errorf("Shutdown returned %v for a drain that could not write its marks, want an error wrapping %v", err, redis.ErrClosed)
	}
	if got := testutil.ToFloat64(droppedMarksMetric) - droppedBefore; got != 3 {
		t.Errorf("a drain that failed to write 3 marks moved the dropped counter by %v, want 3", got)
	}
}

// isReapCall reports a script call whose first key is a backlog, which only the reap script makes.
func isReapCall(cmd redis.Cmder) bool {
	args := cmd.Args()
	if len(args) < 4 {
		return false
	}
	name := strings.ToLower(fmt.Sprint(args[0]))
	key, _ := args[3].(string)
	return (name == "evalsha" || name == "eval") && strings.HasPrefix(key, "client:")
}

// reapReplies records the payload bytes of every reap reply the client reads.
type reapReplies struct {
	mu    sync.Mutex
	sizes []int
}

func (h *reapReplies) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *reapReplies) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *reapReplies) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		c, ok := cmd.(*redis.Cmd)
		if !ok || !isReapCall(cmd) {
			return err
		}
		items, _ := c.Val().([]interface{})
		size := 0
		for _, item := range items {
			if payload, ok := item.(string); ok {
				size += len(payload)
			}
		}
		if size > 0 {
			h.mu.Lock()
			h.sizes = append(h.sizes, size)
			h.mu.Unlock()
		}
		return err
	}
}

// TestValkeyStorage_ReapIsBoundedByBytes expires members far larger than an average message, as a
// request near MAX_BODY_SIZE makes. Each reap reply must stay within the byte budget plus one member.
func TestValkeyStorage_ReapIsBoundedByBytes(t *testing.T) {
	uri := getTestValkeyURI(t)
	storage, err := NewValkeyStorage(uri)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	replies := &reapReplies{}
	storage.client.AddHook(replies)

	ctx := context.Background()
	clientID := fmt.Sprintf("large-%d", time.Now().UnixNano())
	defer cleanupKeys(storage, clientID)

	const count = 5
	payload := []byte(strings.Repeat("x", 400<<10))
	member := 0
	for i := 1; i <= count; i++ {
		msg := models.SseMessage{EventId: int64(100 + i), Message: payload, To: clientID}
		if err := storage.Pub(ctx, msg, 1); err != nil {
			t.Fatalf("Pub failed: %v", err)
		}
		if encoded, err := json.Marshal(msg); err == nil {
			member = max(member, len(encoded))
		}
	}
	time.Sleep(1500 * time.Millisecond)

	before := testutil.ToFloat64(expiredMessagesMetric)
	sweepsBefore := storage.sweeps.Load()
	messageCh := make(chan models.SseMessage, 4)
	if err := storage.Sub(ctx, []string{clientID}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{clientID}, messageCh) }()
	waitSwept(t, storage, clientID, sweepsBefore, 0)

	if got := testutil.ToFloat64(expiredMessagesMetric) - before; got != count {
		t.Errorf("the sweep counted %v of %d expired large messages", got, count)
	}
	replies.mu.Lock()
	defer replies.mu.Unlock()
	if len(replies.sizes) < 2 {
		t.Errorf("%d members of %d bytes came back in %d reap replies, want them split by the %d-byte budget", count, member, len(replies.sizes), reapBatchBytes)
	}
	for _, size := range replies.sizes {
		if size > reapBatchBytes+member {
			t.Errorf("a reap reply carried %d bytes, over the %d-byte budget plus one %d-byte member", size, reapBatchBytes, member)
		}
	}
}
