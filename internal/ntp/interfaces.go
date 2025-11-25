package ntp

// TimeProvider provides the current time in milliseconds.
// This interface allows using either local time or NTP-synchronized time.
type TimeProvider interface {
	NowUnixMilli() int64
}
