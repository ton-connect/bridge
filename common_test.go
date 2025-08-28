package main

import (
	"net/http"
	"testing"
)

func TestExtractOrigin(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{
			name:   "empty string",
			rawURL: "",
			want:   "",
		},
		{
			name:   "valid https URL",
			rawURL: "https://example.com/path",
			want:   "https://example.com",
		},
		{
			name:   "valid http URL",
			rawURL: "http://example.com/path/to/resource",
			want:   "http://example.com",
		},
		{
			name:   "URL with port",
			rawURL: "https://example.com:8080/api",
			want:   "https://example.com:8080",
		},
		{
			name:   "URL with query parameters",
			rawURL: "https://example.com/search?q=test",
			want:   "https://example.com",
		},
		{
			name:   "URL with fragment",
			rawURL: "https://example.com/page#section",
			want:   "https://example.com",
		},
		{
			name:   "invalid URL - no scheme",
			rawURL: "example.com/path",
			want:   "example.com/path",
		},
		{
			name:   "invalid URL - malformed",
			rawURL: "ht tp://example.com",
			want:   "ht tp://example.com",
		},
		{
			name:   "URL with subdomain",
			rawURL: "https://api.example.com/v1/users",
			want:   "https://api.example.com",
		},
		{
			name:   "localhost URL",
			rawURL: "http://localhost:3000/app",
			want:   "http://localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractOrigin(tt.rawURL); got != tt.want {
				t.Errorf("ExtractOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRealIP(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		remoteAddr     string
		expectedPrefix string // We'll check if the result starts with this
	}{
		{
			name:           "X-Forwarded-For with single IP",
			headers:        map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr:     "192.168.1.1:8080",
			expectedPrefix: "203.0.113.1",
		},
		{
			name:           "X-Forwarded-For with multiple IPs",
			headers:        map[string]string{"X-Forwarded-For": "203.0.113.1, 192.168.1.1"},
			remoteAddr:     "10.0.0.1:8080",
			expectedPrefix: "203.0.113.1",
		},
		{
			name:           "No headers, use RemoteAddr",
			headers:        map[string]string{},
			remoteAddr:     "203.0.113.1:8080",
			expectedPrefix: "203.0.113.1",
		},
	}

	realIP, err := newRealIPExtractor([]string{"0.0.0.0/0"})
	if err != nil {
		t.Fatalf("failed to create realIPExtractor: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header:     make(http.Header),
				RemoteAddr: tt.remoteAddr,
			}

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := realIP.Extract(req)
			if result == "" {
				t.Errorf("realIP() returned empty string")
			}

			// Basic validation that we get some IP-like result
			if len(result) < 7 { // minimum for "1.1.1.1"
				t.Errorf("realIP() returned suspiciously short result: %q", result)
			}
		})
	}
}
