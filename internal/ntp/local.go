package ntp

import "time"

// LocalTimeProvider provides time based on the local system clock.
type LocalTimeProvider struct{}

// NewLocalTimeProvider creates a new local time provider.
func NewLocalTimeProvider() *LocalTimeProvider {
	return &LocalTimeProvider{}
}

// NowUnixMilli returns the current local system time in Unix milliseconds.
func (l *LocalTimeProvider) NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
