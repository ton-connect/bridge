package bridge_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var BRIDGE_METRICS_URL = os.Getenv("BRIDGE_METRICS_URL")

func TestBridgeMetrics_CountsRequestsRejectedByBodyLimit(t *testing.T) {
	if BRIDGE_METRICS_URL == "" {
		t.Skip("BRIDGE_METRICS_URL is not set")
	}
	labels := []string{`code="413"`, `method="POST"`, `url="/bridge/message"`}
	before := scrapeCounter(t, "http_requests_total", labels)

	u, err := url.Parse(BRIDGE_URL + "/message")
	if err != nil {
		t.Fatalf("parse bridge url: %v", err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, testHTTPTimeout)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(testHTTPTimeout)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	// net/http will not send a declared length its body does not match, so the request is written by hand.
	if _, err := fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n", u.Path, u.Host, int64(1)<<40); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}

	if after := scrapeCounter(t, "http_requests_total", labels); after < before+1 {
		t.Fatalf("http_requests_total{%s} went from %v to %v, want an increase", strings.Join(labels, ","), before, after)
	}
}

func scrapeCounter(t *testing.T, name string, labels []string) float64 {
	t.Helper()
	resp, err := http.Get(BRIDGE_METRICS_URL)
	if err != nil {
		t.Fatalf("scrape metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scrape metrics: status %d", resp.StatusCode)
	}

	var sum float64
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, name+"{") || !containsAll(line, labels) {
			continue
		}
		value, err := strconv.ParseFloat(line[strings.LastIndexByte(line, ' ')+1:], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		sum += value
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	return sum
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
