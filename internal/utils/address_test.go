package utils

import (
	"testing"
)

func TestNewPublicAddressFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errType error
	}{
		{
			name:    "valid address lowercase",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: false,
		},
		{
			name:    "valid address uppercase",
			input:   "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
			wantErr: false,
		},
		{
			name:    "valid address mixed case",
			input:   "0123456789aBcDeF0123456789aBcDeF0123456789aBcDeF0123456789aBcDeF",
			wantErr: false,
		},
		{
			name:    "valid address with whitespace",
			input:   "  0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  ",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errType: ErrInvalidPublicAddressLength,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
			errType: ErrInvalidPublicAddressLength,
		},
		{
			name:    "too short",
			input:   "0123456789abcdef",
			wantErr: true,
			errType: ErrInvalidPublicAddressLength,
		},
		{
			name:    "too long",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00",
			wantErr: true,
			errType: ErrInvalidPublicAddressLength,
		},
		{
			name:    "invalid character",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg",
			wantErr: true,
			errType: ErrInvalidPublicAddressFormat,
		},
		{
			name:    "invalid character at start",
			input:   "g123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantErr: true,
			errType: ErrInvalidPublicAddressFormat,
		},
		{
			name:    "contains space at end (too long)",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde ",
			wantErr: true,
			errType: ErrInvalidPublicAddressLength,
		},
		{
			name:    "contains space in middle",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd e",
			wantErr: true,
			errType: ErrInvalidPublicAddressFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPublicAddressFromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPublicAddressFromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != tt.errType {
					t.Errorf("NewPublicAddressFromString() error = %v, want %v", err, tt.errType)
				}
			} else {
				// NewPublicAddressFromString validates the trimmed string but returns the original
				if got.String() != tt.input {
					t.Errorf("NewPublicAddressFromString() = %v, want %v", got.String(), tt.input)
				}
			}
		})
	}
}
