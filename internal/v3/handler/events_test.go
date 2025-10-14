package handlerv3

import (
	"sync"
	"testing"

	"github.com/tonkeeper/bridge/internal/utils"
)

func TestEventIDGenerator_NextID(t *testing.T) {
	gen := NewEventIDGenerator()

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
	gen1 := NewEventIDGenerator()
	gen2 := NewEventIDGenerator()

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
	gen := NewEventIDGenerator()
	const numIDs = 1000

	idsChan := make(chan int64, numIDs)
	var wg sync.WaitGroup

	for i := 0; i < numIDs; i++ {
		wg.Add(1)
		utils.RunWithRecovery(func() {
			defer wg.Done()
			id := gen.NextID()
			idsChan <- id
		})
	}

	// Close channel when all goroutines are done
	utils.RunWithRecovery(func() {
		wg.Wait()
		close(idsChan)
	})

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

func TestEventIDGenerator_MultipleGenerators_Ordering(t *testing.T) {
	const numGenerators = 5
	const idsPerGenerator = 100
	const concurrency = 10 // Number of goroutines per generator

	generators := make([]*EventIDGenerator, numGenerators)
	for i := 0; i < numGenerators; i++ {
		generators[i] = NewEventIDGenerator()
	}

	expectedTotal := numGenerators * (idsPerGenerator / concurrency) * concurrency
	idsChan := make(chan int64, expectedTotal)
	var wg sync.WaitGroup

	// Generate IDs from multiple generators with high concurrency
	for _, gen := range generators {
		for c := 0; c < concurrency; c++ {
			wg.Add(1)
			utils.RunWithRecovery(func() {
				defer wg.Done()
				idsPerRoutine := idsPerGenerator / concurrency
				for j := 0; j < idsPerRoutine; j++ {
					id := gen.NextID()
					idsChan <- id
				}
			})
		}
	}

	// Close channel when all goroutines are done
	utils.RunWithRecovery(func() {
		wg.Wait()
		close(idsChan)
	})

	// Collect all IDs from channel
	var allIDs []int64
	for id := range idsChan {
		allIDs = append(allIDs, id)
	}

	// Check for duplicates
	seen := make(map[int64]bool)
	for _, id := range allIDs {
		if seen[id] {
			t.Errorf("Found duplicate ID: %d", id)
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
	maxOutOfOrder := len(allIDs) / 5 // Allow up to 20% out of order
	if reversedOrderCount > maxOutOfOrder {
		t.Errorf("Too many out-of-order IDs: %d (max allowed: %d)", reversedOrderCount, maxOutOfOrder)
	}

	expectedTotal = numGenerators * (idsPerGenerator / concurrency) * concurrency
	if len(allIDs) != expectedTotal {
		t.Errorf("Expected %d IDs, got %d", expectedTotal, len(allIDs))
	}
}
