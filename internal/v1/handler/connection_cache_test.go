package handlerv1

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestConnectionCache_BasicOperations(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	if v := cache.Verify("client1", "127.0.0.1", "https://example.com"); v != "ok" {
		t.Error("Expected verification to return 'ok'")
	}

	if cache.Verify("client2", "127.0.0.1", "https://example.com") != "unknown" {
		t.Error("Expected verification to return 'unknown' with wrong client ID")
	}

	if cache.Verify("client1", "192.168.1.1", "https://example.com") != "warning" {
		t.Error("Expected verification to return 'warning' with wrong IP")
	}

	if cache.Verify("client1", "127.0.0.1", "https://other.com") != "danger" {
		t.Error("Expected verification to return 'danger' with wrong origin")
	}
}

func TestConnectionCache_Expiration(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")

	if cache.Verify("client1", "127.0.0.1", "https://example.com") != "ok" {
		t.Error("Expected verification to return 'ok' immediately")
	}

	time.Sleep(60 * time.Millisecond)

	if v := cache.Verify("client1", "127.0.0.1", "https://example.com"); v != "unknown" {
		t.Errorf("Expected verification to return 'unknown' after expiration, got %s", v)
	}
}

func TestConnectionCache_CapacityLimit(t *testing.T) {
	cache := NewConnectionCache(2, time.Minute)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")
	cache.Add("client3", "127.0.0.3", "https://example.com", "Mozilla/5.0") // Should evict client1

	if cache.Verify("client1", "127.0.0.1", "https://example.com") != "unknown" {
		t.Error("Expected client1 to be evicted and return 'unknown'")
	}

	if cache.Verify("client2", "127.0.0.2", "https://example.com") != "ok" {
		t.Error("Expected client2 to still be in cache and return 'ok'")
	}
	if cache.Verify("client3", "127.0.0.3", "https://example.com") != "ok" {
		t.Error("Expected client3 to still be in cache and return 'ok'")
	}
}

func TestConnectionCache_MultipleConnectionsSameClient(t *testing.T) {
	cache := NewConnectionCache(10, time.Minute)

	// Add multiple connections for same client with different IPs
	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client1", "192.168.1.1", "https://example.com", "Mozilla/5.0")

	// Both connections should be verified as "ok"
	if cache.Verify("client1", "127.0.0.1", "https://example.com") != "ok" {
		t.Error("Expected first connection to return 'ok'")
	}

	if cache.Verify("client1", "192.168.1.1", "https://example.com") != "ok" {
		t.Error("Expected second connection to return 'ok'")
	}

	// Different IP for same client should return "warning"
	if cache.Verify("client1", "10.0.0.1", "https://example.com") != "warning" {
		t.Error("Expected verification with new IP to return 'warning'")
	}

	// Add connection with different origin - should trigger "danger" for new requests
	cache.Add("client1", "127.0.0.1", "https://malicious.com", "Mozilla/5.0")

	if cache.Verify("client1", "127.0.0.1", "https://different.com") != "danger" {
		t.Error("Expected verification with different origin to return 'danger'")
	}
}

func TestConnectionCache_CleanExpired(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")

	if cache.Len() != 2 {
		t.Errorf("Expected cache length to be 2, got %d", cache.Len())
	}

	time.Sleep(60 * time.Millisecond)
	cache.CleanExpired()

	if cache.Len() != 0 {
		t.Errorf("Expected cache length to be 0 after cleanup, got %d", cache.Len())
	}

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	if cache.Len() != 1 {
		t.Errorf("Expected cache length to be 1 after cleanup, got %d", cache.Len())
	}

	time.Sleep(20 * time.Millisecond)
	cache.Add("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client3", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	if cache.Len() != 3 {
		t.Errorf("Expected cache length to be 3 after cleanup, got %d", cache.Len())
	}

	time.Sleep(40 * time.Millisecond)
	cache.CleanExpired()

	if cache.Len() != 2 {
		t.Errorf("Expected cache length to be 2, got %d", cache.Len())
	}

	// check that add moves expire to front
	cache.Add("client2", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client3", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	time.Sleep(40 * time.Millisecond)
	cache.CleanExpired()
	if cache.Len() != 2 {
		t.Errorf("Expected cache length to be 2, got %d", cache.Len())
	}
}

func TestConnectionCache_CleanExpiredWorker(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)
	customCleanupInterval := time.Millisecond * 10
	cache.StartBackgroundCleanup(&customCleanupInterval)

	cache.Add("client1", "127.0.0.1", "https://example.com", "Mozilla/5.0")
	cache.Add("client2", "127.0.0.2", "https://example.com", "Mozilla/5.0")

	if cache.Len() != 2 {
		t.Errorf("Expected cache length to be 2, got %d", cache.Len())
	}

	time.Sleep(60 * time.Millisecond)

	if cache.Len() != 0 {
		t.Errorf("Expected cache length to be 0 after cleanup, got %d", cache.Len())
	}
}

func TestConnectionCache_WrongRemoveOlders(t *testing.T) {
	cache := NewConnectionCache(10, 50*time.Millisecond)

	// Create a custom logger to capture log messages
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	log.SetLevel(log.ErrorLevel)
	defer func() {
		log.SetOutput(os.Stderr) // Reset to default
	}()

	cache.removeOldest()

	// Check that error was logged for removeOldest
	if !strings.Contains(logOutput.String(), "removeOldest called without mutex locked!!!") {
		t.Error("Expected removeOldest error to be logged")
	}

	// Reset log output for next test
	logOutput.Reset()

	cache.removeElement(nil)

	// Check that error was logged for removeElement
	if !strings.Contains(logOutput.String(), "removeElement called without mutex locked!!!") {
		t.Error("Expected removeElement error to be logged")
	}

	if !strings.Contains(logOutput.String(), "removeElement called with nil element!!!") {
		t.Error("Expected removeElement error to be logged")
	}
}

// Essential benchmark tests for ConnectionCache

// Realistic test data for benchmarks
var (
	// Common legitimate origins
	origins = []string{
		"https://app.tonkeeper.com",
		"https://wallet.tonkeeper.com",
		"https://tonhub.com",
		"https://app.tonhub.com",
		"https://openmask.app",
		"https://chrome-extension://nphplpgoakhhjchkkhmiggakijnkhfnd",
		"https://moz-extension://12345678-1234-1234-1234-123456789012",
		"https://getgems.io",
		"https://fragment.com",
		"https://ton.org",
		"https://mytonwallet.org",
		"https://tonapi.io",
		"https://dton.io",
		"https://dedust.io",
		"https://ston.fi",
	}

	// Suspicious/malicious origins for testing
	suspiciousOrigins = []string{
		"https://fake-tonkeeper.com",
		"https://phishing-wallet.net",
		"https://malicious-site.org",
		"https://scam-ton.io",
		"https://evil-extension.com",
	}

	// Common user agents
	userAgents = []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.43 Mobile Safari/537.36",
		"Tonkeeper/3.8.0 (iOS 17.1; iPhone15,2)",
		"Tonkeeper/3.8.0 (Android 14; SM-G998B)",
		"TonHub/4.2.1 (iOS 17.1; iPhone14,3)",
		"MyTonWallet/2.1.0 WebApp",
		"OpenMask/1.5.0 Extension",
	}

	// Suspicious user agents
	// suspiciousUserAgents = []string{
	// 	"PhishingBot/1.0",
	// 	"ScamWallet/0.1",
	// 	"curl/7.68.0",
	// 	"python-requests/2.25.1",
	// 	"FakeApp/1.0.0",
	// }
)

func BenchmarkConnectionCache(b *testing.B) {
	cache := NewConnectionCache(500000, 10*time.Second)

	// Pre-populate with realistic diverse data
	for i := 0; i < 1000; i++ {
		clientID := fmt.Sprintf("prod_client_%d", i)
		ip := fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256)
		origin := origins[i%len(origins)]
		userAgent := userAgents[i%len(userAgents)]
		cache.Add(clientID, ip, origin, userAgent)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clientID := fmt.Sprintf("prod_client_%d", i%1000)
			baseOrigin := origins[i%len(origins)]
			baseUserAgent := userAgents[i%len(userAgents)]

			switch i % 20 {
			case 0, 1, 2, 3: // 20% new connections with legitimate data
				ip := fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256)
				cache.Add(clientID, ip, baseOrigin, baseUserAgent)

			case 4, 5, 6, 7, 8, 9, 10, 11: // 40% normal verification (exact match)
				ip := fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256)
				cache.Verify(clientID, ip, baseOrigin)

			case 12, 13, 14: // 15% suspicious IP changes (warning)
				suspiciousIP := fmt.Sprintf("203.0.113.%d", i%255)
				cache.Verify(clientID, suspiciousIP, baseOrigin)

			case 15, 16: // 10% different user agent (danger)
				ip := fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256)
				cache.Verify(clientID, ip, baseOrigin)

			case 17, 18: // 10% different origin (danger)
				ip := fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256)
				maliciousOrigin := suspiciousOrigins[i%len(suspiciousOrigins)]
				cache.Verify(clientID, ip, maliciousOrigin)

			case 19: // 5% completely unknown clients (unknown)
				unknownClient := fmt.Sprintf("attacker_%d", i)
				unknownIP := fmt.Sprintf("1.2.3.%d", i%255)
				maliciousOrigin := suspiciousOrigins[i%len(suspiciousOrigins)]
				cache.Verify(unknownClient, unknownIP, maliciousOrigin)
			}
			i++
		}
	})
}
