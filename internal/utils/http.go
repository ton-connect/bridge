package utils

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/realclientip/realclientip-go"
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

type RealIPExtractor struct {
	strategy realclientip.RightmostTrustedRangeStrategy
}

// NewRealIPExtractor creates a new realIPExtractor with the given trusted ranges.
func NewRealIPExtractor(trustedRanges []string) (*RealIPExtractor, error) {
	ipNets, err := realclientip.AddressesAndRangesToIPNets(trustedRanges...)
	if err != nil {
		return nil, err
	}

	strategy, err := realclientip.NewRightmostTrustedRangeStrategy("X-Forwarded-For", ipNets)
	if err != nil {
		return nil, err
	}

	return &RealIPExtractor{
		strategy: strategy,
	}, nil
}

var remoteAddrStrategy = realclientip.RemoteAddrStrategy{}

func (e *RealIPExtractor) Extract(request *http.Request) string {
	headers := request.Header.Clone()

	newXForwardedFor := []string{}
	oldXForwardedFor := headers.Get("X-Forwarded-For")

	if oldXForwardedFor != "" {
		newXForwardedFor = append(newXForwardedFor, oldXForwardedFor)
	}

	remoteAddr := remoteAddrStrategy.ClientIP(nil, request.RemoteAddr)
	if remoteAddr == "" || len(newXForwardedFor) == 0 {
		return remoteAddr
	}

	newXForwardedFor = append(newXForwardedFor, remoteAddr)
	headers.Set("X-Forwarded-For", strings.Join(newXForwardedFor, ", "))

	// RightmostTrustedRangeStrategy ignore the second parameter
	rightmostTrusted := e.strategy.ClientIP(headers, "")
	if rightmostTrusted == "" {
		return remoteAddr
	}
	return rightmostTrusted
}
