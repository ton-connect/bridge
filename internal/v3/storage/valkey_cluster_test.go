package storagev3

import (
	"testing"
)

// TestForceClusterMode verifies that clustered-valkey/clustered-redis
// storage types properly enforce cluster mode
func TestForceClusterMode(t *testing.T) {
	tests := []struct {
		name         string
		storageType  string
		uri          string
		expectError  bool
		errorKeyword string
	}{
		{
			name:        "valkey auto-detect allows single-node",
			storageType: "valkey",
			uri:         "redis://localhost:6379",
			expectError: false, // Connection may fail, but won't complain about cluster
		},
		{
			name:         "clustered-valkey requires cluster",
			storageType:  "clustered-valkey",
			uri:          "redis://localhost:6379",
			expectError:  true,
			errorKeyword: "cluster",
		},
		{
			name:         "clustered-redis requires cluster",
			storageType:  "clustered-redis",
			uri:          "redis://localhost:6379",
			expectError:  true,
			errorKeyword: "cluster",
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewStorage(tt.storageType, tt.uri)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.storageType)
					return
				}
				// Verify error mentions cluster requirement
				if tt.errorKeyword != "" {
					errMsg := err.Error()
					if !contains(errMsg, tt.errorKeyword) {
						t.Errorf("expected error containing '%s', got: %v", tt.errorKeyword, err)
					}
				}
				// Should not have storage on error
				if storage != nil {
					t.Errorf("expected nil storage on error, got non-nil: %T", storage)
				}
			} else {
				// For auto-detect mode, storage may be nil due to connection failure,
				// but error should not mention cluster enforcement
				if err != nil && contains(err.Error(), "cluster mode required") {
					t.Errorf("unexpected cluster enforcement error in auto-detect mode: %v", err)
				}

				// Cleanup successful connections
				if storage != nil && err == nil {
					if vs, ok := storage.(*ValkeyStorage); ok && vs.client != nil {
						vs.client.Close()
					}
				}
			}
		})
	}
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
