package utils

import (
	log "github.com/sirupsen/logrus"
)

// RunWithRecovery runs a function in a goroutine with panic recovery.
// It logs any recovered panics and continues execution.
func RunWithRecovery(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("RECOVERED FROM PANIC: %v", r)
			}
		}()
		fn()
	}()
}
