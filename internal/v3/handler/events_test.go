package handlerv3

import (
	"sync"
	"testing"
)

func TestEventIDGenerator_NextID(t *testing.T) {
	gen := NewEventIDGenerator(nil)

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
	gen1 := NewEventIDGenerator(nil)
	gen2 := NewEventIDGenerator(nil)

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
	gen := NewEventIDGenerator(nil)
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
