package handlerv3

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ton-connect/bridge/internal/ntp"
)

func TestEventIDGenerator_NextID(t *testing.T) {
	gen := NewEventIDGenerator(ntp.NewLocalTimeProvider())

	id1 := gen.NextID()
	id2 := gen.NextID()

	if id1 == 0 || id2 == 0 {
		t.Error("IDs should not be zero")
	}
	if id2 <= id1 {
		t.Error("IDs should be increasing")
	}
}

func TestEventIDGenerator_RandomOffset(t *testing.T) {
	gen1 := NewEventIDGenerator(ntp.NewLocalTimeProvider())
	gen2 := NewEventIDGenerator(ntp.NewLocalTimeProvider())

	// Generators should have different offsets
	if gen1.offset == gen2.offset {
		t.Error("Different generators should have different random offsets (very unlikely but possible)")
	}

	// IDs from different generators should be different even if generated quickly
	id1 := gen1.NextID()
	id2 := gen2.NextID()

	if id1 == id2 {
		t.Error("IDs from different generators should be different due to random offset")
	}
}

func TestEventIDGenerator_SingleGenerators_Ordering(t *testing.T) {
	gen := NewEventIDGenerator(ntp.NewLocalTimeProvider())
	const numIDs = 1000

	idsChan := make(chan int64, numIDs)
	var wg sync.WaitGroup

	for i := 0; i < numIDs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := gen.NextID()
			idsChan <- id
		}()
	}

	// Close channel when all goroutines are done
	go func() {
		wg.Wait()
		close(idsChan)
	}()

	// Collect all IDs from channel
	var allIDs []int64
	for id := range idsChan {
		allIDs = append(allIDs, id)
	}

	// Check for duplicates
	seen := make(map[int64]bool)
	for _, id := range allIDs {
		if seen[id] {
			t.Error("Found duplicate ID")
		}
		seen[id] = true
	}

	// Verify IDs are mostly ordered by timestamp
	reversedOrderCount := 0
	for i := 1; i < len(allIDs); i++ {
		if allIDs[i] < allIDs[i-1] {
			reversedOrderCount++
		}
	}

	// Allow some out-of-order IDs due to concurrent generation
	// but most should be in timestamp order
	maxOutOfOrder := len(allIDs) / 3 // Allow up to 33% out of order TODO fix it
	if reversedOrderCount > maxOutOfOrder {
		t.Errorf("Too many out-of-order IDs: %d (max allowed: %d)", reversedOrderCount, maxOutOfOrder)
	}
}

func TestGetIdFromParams_TimestampEncoding(t *testing.T) {
	// Test timestamps that fit within 42 bits (up to ~year 2109 from Unix epoch)
	// With 53 bits total and 11 bits for nonce, we have 42 bits for timestamp
	// These should round-trip exactly
	testCasesExact := []struct {
		name      string
		timestamp int64
	}{
		{"Unix epoch", 0},
		{"Year 1970", time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{"Year 2000", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{"Year 2024", time.Date(2024, 12, 2, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{"Year 2050", time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{"Year 2100", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{"Max 42-bit timestamp", int64(1<<42 - 1)},
	}

	for _, tc := range testCasesExact {
		t.Run(tc.name, func(t *testing.T) {
			id := getIdFromParams(tc.timestamp, 0)

			// Extract timestamp from ID (upper 42 bits)
			extractedTimestamp := id >> 11

			if extractedTimestamp != tc.timestamp {
				t.Errorf("Timestamp mismatch: got %d, want %d", extractedTimestamp, tc.timestamp)
			}

			// Verify ID is within 53-bit range
			max53Bit := int64(0x1FFFFFFFFFFFFF) // 2^53 - 1
			if id > max53Bit {
				t.Errorf("ID %d exceeds 53-bit limit %d", id, max53Bit)
			}
		})
	}
}

func TestGetIdFromParams_NonceEncoding(t *testing.T) {
	timestamp := time.Date(2024, 12, 2, 0, 0, 0, 0, time.UTC).UnixMilli()

	testCases := []struct {
		name          string
		nonce         int64
		expectedNonce int64 // Expected after 11-bit masking
	}{
		{"Zero nonce", 0, 0},
		{"Nonce 1", 1, 1},
		{"Nonce 100", 100, 100},
		{"Nonce 1000", 1000, 1000},
		{"Max 11-bit nonce", 0x7FF, 0x7FF},
		{"Overflow nonce 0x800", 0x800, 0},            // Should wrap to 0
		{"Overflow nonce 0x801", 0x801, 1},            // Should wrap to 1
		{"Large nonce", 123456789, 123456789 & 0x7FF}, // Should be masked
		{"Very large nonce", 9999999999, 9999999999 & 0x7FF},
		{"Random big number", 0x7FFFFFFFFFFFFFFF, 0x7FFFFFFFFFFFFFFF & 0x7FF},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id := getIdFromParams(timestamp, tc.nonce)

			// Extract nonce from ID (lower 11 bits)
			extractedNonce := id & 0x7FF

			if extractedNonce != tc.expectedNonce {
				t.Errorf("Nonce mismatch: got %d, want %d", extractedNonce, tc.expectedNonce)
			}

			// Verify timestamp is preserved
			extractedTimestamp := id >> 11
			if extractedTimestamp != timestamp {
				t.Errorf("Timestamp was corrupted: got %d, want %d", extractedTimestamp, timestamp)
			}
		})
	}
}

func TestGetIdFromParams_RoundTrip(t *testing.T) {
	// Test that we can encode and decode many random combinations
	r := rand.New(rand.NewSource(42))

	// Maximum timestamp that fits in 42 bits (2^42 - 1 ms ≈ 139 years from epoch)
	// After 53-bit masking, timestamps up to 42 bits are preserved exactly
	max42BitTimestamp := int64(1<<42 - 1)

	for i := 0; i < 10; i++ {
		// Random timestamp within 42-bit range
		timestamp := r.Int63n(max42BitTimestamp)
		// Random nonce (can be any large number, will be masked to 11 bits)
		nonce := r.Int63()
		expectedNonce := nonce & 0x7FF

		id := getIdFromParams(timestamp, nonce)

		// Decode
		extractedTimestamp := id >> 11
		extractedNonce := id & 0x7FF

		if extractedTimestamp != timestamp {
			t.Errorf("Iteration %d: Timestamp mismatch: got %d, want %d", i, extractedTimestamp, timestamp)
		}
		if extractedNonce != expectedNonce {
			t.Errorf("Iteration %d: Nonce mismatch: got %d, want %d (original: %d)", i, extractedNonce, expectedNonce, nonce)
		}

		// Verify within 53-bit range
		max53Bit := int64(0x1FFFFFFFFFFFFF)
		if id > max53Bit {
			t.Errorf("Iteration %d: ID %d exceeds 53-bit limit", i, id)
		}
	}
}

func TestGetIdFromParams_53BitLimit(t *testing.T) {
	// Test edge case: maximum 42-bit timestamp
	max42BitTimestamp := int64(1<<42 - 1)
	maxNonce := int64(0x7FF)

	id := getIdFromParams(max42BitTimestamp, maxNonce)

	// Should be exactly 2^53 - 1
	expectedMax := int64(0x1FFFFFFFFFFFFF)
	if id != expectedMax {
		t.Errorf("Max ID mismatch: got %d (0x%X), want %d (0x%X)", id, id, expectedMax, expectedMax)
	}

	// Test that timestamps beyond 42 bits get truncated by the final mask
	beyond42BitTimestamp := int64(1 << 43)
	id2 := getIdFromParams(beyond42BitTimestamp, 0)

	// Should be truncated to fit within 53 bits
	if id2 > expectedMax {
		t.Errorf("ID with large timestamp exceeds 53 bits: %d", id2)
	}
}

func TestGetIdFromParams_Monotonicity(t *testing.T) {
	// IDs should increase when timestamp increases (with same nonce)
	timestamp1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	timestamp2 := time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC).UnixMilli() // 1 second later

	id1 := getIdFromParams(timestamp1, 0)
	id2 := getIdFromParams(timestamp2, 0)

	if id2 <= id1 {
		t.Errorf("ID should increase with timestamp: id1=%d, id2=%d", id1, id2)
	}

	// IDs should increase when nonce increases (with same timestamp)
	id3 := getIdFromParams(timestamp1, 1)
	id4 := getIdFromParams(timestamp1, 2)

	if id4 <= id3 {
		t.Errorf("ID should increase with nonce: id3=%d, id4=%d", id3, id4)
	}
}
