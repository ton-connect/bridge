package storagev3

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

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

func testClientKey(t *testing.T, i int) string {
	t.Helper()
	return fmt.Sprintf("%s-%s-%d", t.Name(), rand.Text(), i)
}

// countDeliveries counts the copies of eventID on messageCh: it waits up to wait for the first one,
// then settle longer for a second.
func countDeliveries(messageCh <-chan models.SseMessage, eventID int64, wait, settle time.Duration) int {
	n := 0
	deadline := time.After(wait)
	for {
		select {
		case msg := <-messageCh:
			if msg.EventId == eventID {
				n++
				if n == 1 {
					deadline = time.After(settle)
				}
			}
		case <-deadline:
			return n
		}
	}
}

// deliveryFailures runs rounds of Sub on a fresh key with one Pub to it, published while Sub runs
// when concurrent is set and right after it returns otherwise.
func deliveryFailures(t *testing.T, storage *ValkeyStorage, rounds int, concurrent bool) (lost, duplicated int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < rounds; i++ {
		key := testClientKey(t, i)
		messageCh := make(chan models.SseMessage, 10)
		msg := models.SseMessage{EventId: int64(i + 1), Message: []byte("m"), To: key}
		published := make(chan error, 1)
		if concurrent {
			go func() {
				time.Sleep(time.Duration(i%4) * 500 * time.Microsecond)
				published <- storage.Pub(ctx, msg, 60)
			}()
		}
		if err := storage.Sub(ctx, []string{key}, 0, messageCh); err != nil {
			t.Fatalf("Sub failed: %v", err)
		}
		if !concurrent {
			published <- storage.Pub(ctx, msg, 60)
		}
		if err := <-published; err != nil {
			t.Fatalf("Pub failed: %v", err)
		}
		switch n := countDeliveries(messageCh, msg.EventId, time.Second, 50*time.Millisecond); {
		case n == 0:
			lost++
		case n > 1:
			duplicated++
		}
		_ = storage.Unsub(ctx, []string{key}, messageCh)
	}
	return lost, duplicated
}

// delayedConn holds back the writes that carry the named commands, keeping write order, the way a
// busy node or a slow link does. Write returns at once, so the caller does not wait out the delay.
type delayedConn struct {
	net.Conn
	delays map[string]time.Duration
	queue  chan delayedWrite
	mu     sync.Mutex
	closed bool
}

type delayedWrite struct {
	data []byte
	due  time.Time
}

func (c *delayedConn) Write(b []byte) (int, error) {
	due := time.Now()
	for command, delay := range c.delays {
		if bytes.Contains(b, []byte("\r\n"+command+"\r\n")) {
			due = due.Add(delay)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	c.queue <- delayedWrite{data: bytes.Clone(b), due: due}
	return len(b), nil
}

func (c *delayedConn) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	return c.Conn.Close()
}

func newDelayedValkeyStorage(t *testing.T, delays map[string]time.Duration) *ValkeyStorage {
	t.Helper()
	opts, err := redis.ParseURL(getTestValkeyURI(t))
	if err != nil {
		t.Fatalf("failed to parse URI: %v", err)
	}
	options := clusterOptions(opts)
	dial := redis.NewDialer(&redis.Options{DialTimeout: options.DialTimeout, TLSConfig: options.TLSConfig})
	options.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		c := &delayedConn{Conn: conn, delays: delays, queue: make(chan delayedWrite, 1024)}
		go func() {
			for w := range c.queue {
				time.Sleep(time.Until(w.due))
				if _, err := c.Conn.Write(w.data); err != nil {
					return
				}
			}
		}()
		return c, nil
	}
	client := redis.NewClusterClient(options)
	t.Cleanup(func() { _ = client.Close() })
	return newValkeyStorage(client)
}

func TestValkeyStorage_SubThenPubDelivers(t *testing.T) {
	storage, err := NewValkeyStorage(getTestValkeyURI(t))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if lost, duplicated := deliveryFailures(t, storage, 100, false); lost > 0 || duplicated > 0 {
		t.Errorf("of 100 messages published right after Sub returned, %d were lost and %d delivered twice", lost, duplicated)
	}
}

func TestValkeyStorage_PubAroundSubDeliversExactlyOnce(t *testing.T) {
	storage, err := NewValkeyStorage(getTestValkeyURI(t))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if lost, duplicated := deliveryFailures(t, storage, 100, true); lost > 0 || duplicated > 0 {
		t.Errorf("of 100 messages published while Sub ran, %d were lost and %d delivered twice", lost, duplicated)
	}
}

func TestValkeyStorage_SubWaitsForLateSubscribe(t *testing.T) {
	storage := newDelayedValkeyStorage(t, map[string]time.Duration{"subscribe": 20 * time.Millisecond})
	if lost, duplicated := deliveryFailures(t, storage, 20, false); lost > 0 || duplicated > 0 {
		t.Errorf("with SUBSCRIBE reaching the server late, of 20 messages published right after Sub returned, %d were lost and %d delivered twice", lost, duplicated)
	}
}

func TestValkeyStorage_SubKeepsProbeTimeoutWhenPublishHangs(t *testing.T) {
	storage := newDelayedValkeyStorage(t, map[string]time.Duration{"publish": 3 * subscribeProbeTimeout})
	messageCh := make(chan models.SseMessage, 10)
	start := time.Now()
	if err := storage.Sub(context.Background(), []string{testClientKey(t, 0)}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > subscribeProbeTimeout+time.Second {
		t.Errorf("with PUBLISH unanswered, Sub took %s, past its %s probe timeout", elapsed, subscribeProbeTimeout)
	}
}

func TestValkeyStorage_PubFailsWithoutPublishingWhenStoreFails(t *testing.T) {
	storage, err := NewValkeyStorage(getTestValkeyURI(t))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	ctx := context.Background()
	key := testClientKey(t, 0)
	messageCh := make(chan models.SseMessage, 10)
	if err := storage.Sub(ctx, []string{key}, 0, messageCh); err != nil {
		t.Fatalf("Sub failed: %v", err)
	}
	defer func() { _ = storage.Unsub(ctx, []string{key}, messageCh) }()
	// A string under the channel key makes ZADD answer WRONGTYPE.
	if err := storage.client.Set(ctx, "client:"+key, "not a sorted set", time.Minute).Err(); err != nil {
		t.Fatalf("SET failed: %v", err)
	}

	if err := storage.Pub(ctx, models.SseMessage{EventId: 1, Message: []byte("m"), To: key}, 60); err == nil {
		t.Fatal("Pub returned nil although ZADD failed")
	}
	if n := countDeliveries(messageCh, 1, 500*time.Millisecond, 0); n != 0 {
		t.Errorf("a message Pub reported as failed was delivered live %d time(s)", n)
	}
}

func TestValkeyStorage_PubStoresBeforePublishing(t *testing.T) {
	storage := newDelayedValkeyStorage(t, map[string]time.Duration{
		"subscribe": 20 * time.Millisecond,
		"zadd":      50 * time.Millisecond,
	})
	if lost, duplicated := deliveryFailures(t, storage, 20, true); lost > 0 || duplicated > 0 {
		t.Errorf("with SUBSCRIBE and ZADD reaching the server late, of 20 messages published while Sub ran, %d were lost and %d delivered twice", lost, duplicated)
	}
}

func TestValkeySubscriberSendsHistoryFirstAndEachEventOnce(t *testing.T) {
	messageCh := make(chan models.SseMessage, 10)
	sub := newValkeySubscriber(messageCh)
	msg := func(id int64, payload string) models.SseMessage {
		return models.SseMessage{EventId: id, Message: []byte(payload)}
	}

	sub.deliverLive(msg(2, "b"))
	sub.deliverLive(msg(7, "x"))
	sub.deliverLive(msg(7, "y"))
	sub.deliverReplay(msg(1, "a"))
	sub.deliverReplay(msg(7, "x"))
	sub.deliverReplay(msg(7, "y"))
	sub.replayDone()
	sub.deliverLive(msg(1, "a"))
	sub.deliverLive(msg(3, "c"))
	close(messageCh)

	var got []string
	for m := range messageCh {
		got = append(got, fmt.Sprintf("%d:%s", m.EventId, m.Message))
	}
	want := []string{"1:a", "7:x", "7:y", "2:b", "3:c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("delivered %v, want %v", got, want)
	}
}

func TestValkeySubscriberKeepsSameEventForDifferentRecipients(t *testing.T) {
	messageCh := make(chan models.SseMessage, 10)
	sub := newValkeySubscriber(messageCh)

	sub.deliverLive(models.SseMessage{EventId: 5, Message: []byte("m"), To: "a"})
	sub.deliverLive(models.SseMessage{EventId: 5, Message: []byte("m"), To: "b"})
	sub.deliverReplay(models.SseMessage{EventId: 5, Message: []byte("m"), To: "a"})
	sub.replayDone()
	close(messageCh)

	var got []string
	for m := range messageCh {
		got = append(got, fmt.Sprintf("%d:%s", m.EventId, m.To))
	}
	if want := []string{"5:a", "5:b"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("delivered %v, want %v", got, want)
	}
}
