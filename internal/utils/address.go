package utils

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidPublicAddressFormat is returned when a public address contains
	// characters that are not valid hexadecimal digits (0-9, a-f, A-F).
	ErrInvalidPublicAddressFormat = errors.New("public address must be a valid hex string")

	// ErrInvalidPublicAddressLength is returned when a public address is not
	// exactly 64 characters long.
	ErrInvalidPublicAddressLength = errors.New("public address must be 64 characters long")
)

// PublicAddress represents a 64-character hexadecimal public address.
// It must contain only valid hex characters (0-9, a-f, A-F) and be exactly
// 64 characters in length.
type PublicAddress string

// NewPublicAddressFromString creates a new PublicAddress from a string.
// The input string is trimmed of leading and trailing whitespace for validation,
// but the original string (with whitespace) is returned if valid.
// It returns an error if the address is invalid (wrong length or contains
// non-hexadecimal characters).
func NewPublicAddressFromString(a string) (PublicAddress, error) {
	if err := PublicAddress(strings.TrimSpace(a)).Validate(); err != nil {
		return "", err
	}
	return PublicAddress(a), nil
}

// String returns the string representation of the PublicAddress.
func (a PublicAddress) String() string {
	return string(a)
}

// Validate checks if the PublicAddress is valid.
// It returns ErrInvalidPublicAddressLength if the address is not exactly
// 64 characters long, or ErrInvalidPublicAddressFormat if it contains
// non-hexadecimal characters.
func (a PublicAddress) Validate() error {
	if len(a) != 64 {
		return ErrInvalidPublicAddressLength
	}

	if !a.validateHex() {
		return ErrInvalidPublicAddressFormat
	}

	return nil
}

// validateHex checks if all characters in the PublicAddress are valid
// hexadecimal digits (0-9, a-f, A-F). Returns true if all characters
// are valid hex digits, false otherwise.
func (a PublicAddress) validateHex() bool {
	for i := 0; i < len(a); i++ {
		c := a[i]
		if (c < '0' || c > '9') &&
			(c < 'a' || c > 'f') &&
			(c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
