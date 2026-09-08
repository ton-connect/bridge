package antiscam

// NoopChecker is a DomainChecker that never blocks anything.
// Used when antiscam filtering is disabled.
type NoopChecker struct{}

func (n *NoopChecker) IsBlocked(origin string) bool {
	return false
}
