package utils

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
			name:   "valid http URL but not valid Origin",
			rawURL: "http://example.com/path/to/resource",
			want:   "http://example.com",
		},
		{
			name:   "URL with port",
			rawURL: "https://example.com:8080/api",
			want:   "https://example.com:8080",
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
			rawURL: "https://api.example.com/",
			want:   "https://api.example.com",
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

func TestRealIPExtractor(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		remoteAddr    string
		trustedRanges []string
		want          string
	}{
		{
			name:          "X-Forwarded-For with single IP - trusted proxy",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr:    "192.168.1.1",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "203.0.113.1",
		},
		{
			name:          "X-Forwarded-For with multiple IPs - trusted proxy",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1, 192.168.1.1"},
			remoteAddr:    "192.168.1.2",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "203.0.113.1",
		},
		{
			name:          "No X-Forwarded-For header, use RemoteAddr",
			headers:       map[string]string{},
			remoteAddr:    "203.0.113.1",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "203.0.113.1",
		},
		{
			name:          "Untrusted X-Forwarded-For and RemoteAddr - uses RemoteAddr",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr:    "192.168.1.1",
			trustedRanges: []string{"10.0.0.0/8"},
			want:          "192.168.1.1",
		},
		{
			name:          "Untrusted X-Forwarded-For and trusted RemoteAddr",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr:    "10.0.0.1",
			trustedRanges: []string{"10.0.0.0/8"},
			want:          "203.0.113.1",
		},
		{
			name:          "X-Real-IP header ignored (not supported)",
			headers:       map[string]string{"X-Real-IP": "203.0.113.5"},
			remoteAddr:    "192.168.1.2",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "192.168.1.2",
		},
		{
			name:          "Long proxy chain - 5 IPs",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1, 198.51.100.1, 192.168.1.5, 192.168.1.10, 10.0.0.5"},
			remoteAddr:    "192.168.1.1",
			trustedRanges: []string{"192.168.1.0/24", "10.0.0.0/8"},
			want:          "198.51.100.1",
		},
		{
			name:          "Very long proxy chain - 10 IPs",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.50, 198.51.100.25, 172.16.0.1, 10.1.1.1, 10.2.2.2, 192.168.10.1, 192.168.20.1, 172.17.0.1, 10.0.1.1, 192.168.1.100"},
			remoteAddr:    "192.168.1.1",
			trustedRanges: []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"},
			want:          "198.51.100.25",
		},
		{
			name:          "Proxy chain with mixed trusted/untrusted - returns rightmost untrusted",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.100, 8.8.8.8, 192.168.1.50, 10.0.0.25"},
			remoteAddr:    "192.168.1.1",
			trustedRanges: []string{"192.168.1.0/24", "10.0.0.0/8"},
			want:          "8.8.8.8",
		},
		{
			name:          "IPv6 RemoteAddr",
			headers:       map[string]string{},
			remoteAddr:    "[2001:db8::1]",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "2001:db8::1",
		},
		{
			name:          "Empty RemoteAddr with X-Forwarded-For",
			headers:       map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr:    "",
			trustedRanges: []string{"192.168.1.0/24"},
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realIP, err := NewRealIPExtractor(tt.trustedRanges)
			if err != nil {
				t.Fatalf("failed to create realIPExtractor: %v", err)
			}

			req := &http.Request{
				Header:     make(http.Header),
				RemoteAddr: tt.remoteAddr,
			}

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := realIP.Extract(req)
			if result != tt.want {
				t.Errorf("realIP.Extract() = %q, want %q", result, tt.want)
			}
		})
	}
}
