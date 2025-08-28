package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/realclientip/realclientip-go"
	"github.com/tonkeeper/bridge/config"
)

type HttpRes struct {
	Message    string `json:"message,omitempty" example:"status ok"`
	StatusCode int    `json:"statusCode,omitempty" example:"200"`
}

func HttpResOk() HttpRes {
	return HttpRes{
		Message:    "OK",
		StatusCode: http.StatusOK,
	}
}

func HttpResError(errMsg string, statusCode int) (int, HttpRes) {
	return statusCode, HttpRes{
		Message:    errMsg,
		StatusCode: statusCode,
	}
}

func ExtractOrigin(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Scheme == "" || u.Host == "" {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
}

// realIP extracts the real client IP using RightmostTrustedRangeStrategy
func realIP(request *http.Request) string {
	ranges := config.Config.TrustedProxyRanges
	if len(ranges) == 0 {
		ranges = []string{"0.0.0.0/0"} // fallback to trust all if not configured
	}

	ipNets, err := realclientip.AddressesAndRangesToIPNets(ranges...)
	if err != nil {
		return strings.Split(request.RemoteAddr, ":")[0]
	}

	strategy, err := realclientip.NewRightmostTrustedRangeStrategy("X-Forwarded-For", ipNets)
	if err == nil {
		if ip := strategy.ClientIP(request.Header, request.RemoteAddr); ip != "" {
			return ip
		}
	}

	return strings.Split(request.RemoteAddr, ":")[0]
}
