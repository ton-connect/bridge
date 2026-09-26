package antiscam

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlocklistParsing(t *testing.T) {
	body := "evil.com\nscam.org\n# comment\n\n  spaces.io  \nMIXED.Case\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body) //nolint:errcheck
	}))
	defer srv.Close()

	bl := NewBlocklist(srv.URL, time.Hour)
	bl.Start(context.Background())
	defer bl.Stop()

	cases := []struct {
		origin  string
		blocked bool
	}{
		{"https://evil.com", true},
		{"https://scam.org", true},
		{"https://spaces.io", true},
		{"https://mixed.case", true},
		{"https://MIXED.CASE", true},
		{"https://safe.com", false},
	}
	for _, tc := range cases {
		got := bl.IsBlocked(tc.origin)
		if got != tc.blocked {
			t.Errorf("IsBlocked(%q) = %v, want %v", tc.origin, got, tc.blocked)
		}
	}
}

func TestBlocklistSubdomainMatching(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "evil.com\n") //nolint:errcheck
	}))
	defer srv.Close()

	bl := NewBlocklist(srv.URL, time.Hour)
	bl.Start(context.Background())
	defer bl.Stop()

	cases := []struct {
		origin  string
		blocked bool
	}{
		{"https://evil.com", true},
		{"https://sub.evil.com", true},
		{"https://deep.sub.evil.com", true},
		{"https://notevil.com", false},
		{"https://evil.com.safe.org", false},
		{"", false},
	}
	for _, tc := range cases {
		got := bl.IsBlocked(tc.origin)
		if got != tc.blocked {
			t.Errorf("IsBlocked(%q) = %v, want %v", tc.origin, got, tc.blocked)
		}
	}
}

func TestBlocklistOriginExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "evil.com\n") //nolint:errcheck
	}))
	defer srv.Close()

	bl := NewBlocklist(srv.URL, time.Hour)
	bl.Start(context.Background())
	defer bl.Stop()

	cases := []struct {
		origin  string
		blocked bool
	}{
		{"https://evil.com", true},
		{"https://evil.com:8080", true},
		{"http://evil.com/path?q=1", true},
		{"evil.com", true},
		{"evil.com:443", true},
		{"", false},
	}
	for _, tc := range cases {
		got := bl.IsBlocked(tc.origin)
		if got != tc.blocked {
			t.Errorf("IsBlocked(%q) = %v, want %v", tc.origin, got, tc.blocked)
		}
	}
}

func TestBlocklistRefreshKeepsOldSetOnFailure(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			fmt.Fprint(w, "evil.com\n") //nolint:errcheck
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	bl := NewBlocklist(srv.URL, 50*time.Millisecond)
	bl.Start(context.Background())
	defer bl.Stop()

	if !bl.IsBlocked("https://evil.com") {
		t.Fatal("expected evil.com to be blocked after initial load")
	}

	// Wait for a failed refresh
	time.Sleep(150 * time.Millisecond)

	// Old set should still be active
	if !bl.IsBlocked("https://evil.com") {
		t.Fatal("expected evil.com to still be blocked after failed refresh")
	}
}

func TestNoopChecker(t *testing.T) {
	checker := &NoopChecker{}
	if checker.IsBlocked("https://evil.com") {
		t.Error("NoopChecker should never block")
	}
	if checker.IsBlocked("") {
		t.Error("NoopChecker should never block")
	}
}
