package handles

import (
	"sync"
	"testing"
)

func TestRegisterAndLookup(t *testing.T) {
	type testData struct {
		Name  string
		Value int
	}

	data := &testData{Name: "test", Value: 42}
	handle := Register(data)

	if handle == 0 {
		t.Error("Register should return non-zero handle")
	}

	got := Lookup(handle)
	if got == nil {
		t.Error("Lookup should return non-nil value")
	}

	gotData, ok := got.(*testData)
	if !ok {
		t.Errorf("Lookup returned wrong type: %T", got)
	}

	if gotData.Name != "test" || gotData.Value != 42 {
		t.Errorf("Lookup returned wrong data: %+v", gotData)
	}
}

func TestUnregister(t *testing.T) {
	data := "test string"
	handle := Register(data)

	// Verify it's registered
	if Lookup(handle) == nil {
		t.Error("Expected value before Unregister")
	}

	// Unregister
	Unregister(handle)

	// Verify it's gone
	if Lookup(handle) != nil {
		t.Error("Expected nil after Unregister")
	}
}

func TestLookupNonExistent(t *testing.T) {
	got := Lookup(999999)
	if got != nil {
		t.Error("Lookup of non-existent handle should return nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	const numGoroutines = 100
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				data := struct {
					ID  int
					Seq int
				}{id, j}
				handle := Register(&data)
				got := Lookup(handle)
				if got == nil {
					t.Errorf("Lookup returned nil for handle %d", handle)
				}
				Unregister(handle)
			}
		}(i)
	}

	wg.Wait()
}

func TestHandlesAreUnique(t *testing.T) {
	handles := make(map[uintptr]bool)

	for i := 0; i < 1000; i++ {
		h := Register(i)
		if handles[h] {
			t.Errorf("Handle %d was returned twice", h)
		}
		handles[h] = true
	}

	// Clean up
	for h := range handles {
		Unregister(h)
	}
}
