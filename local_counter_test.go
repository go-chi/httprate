package httprate

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestLocalCounter(t *testing.T) {
	limitCounter := &localCounter{
		latestWindow:     time.Now().UTC().Truncate(time.Second),
		latestCounters:   make(map[uint64]int),
		previousCounters: make(map[uint64]int),
		windowLength:     time.Second,
	}

	// Time = NOW()
	currentWindow := time.Now().UTC().Truncate(time.Second)
	previousWindow := currentWindow.Add(-time.Second)

	for i := 0; i < 5; i++ {
		curr, prev, _ := limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
		if curr != 0 {
			t.Errorf("unexpected curr = %v, expected %v", curr, 0)
		}
		if prev != 0 {
			t.Errorf("unexpected prev = %v, expected %v", prev, 0)
		}

		_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, 1)
		_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, 99)

		curr, prev, _ = limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
		if curr != 100 {
			t.Errorf("unexpected curr = %v, expected %v", curr, 100)
		}
		if prev != 0 {
			t.Errorf("unexpected prev = %v, expected %v", prev, 0)
		}
	}

	// Time++
	currentWindow = currentWindow.Add(time.Second)
	previousWindow = previousWindow.Add(time.Second)

	for i := 0; i < 5; i++ {
		curr, prev, _ := limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
		if curr != 0 {
			t.Errorf("unexpected curr = %v, expected %v", curr, 0)
		}
		if prev != 100 {
			t.Errorf("unexpected prev = %v, expected %v", prev, 100)
		}
		_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, 50)
	}

	// Time++
	currentWindow = currentWindow.Add(time.Second)
	previousWindow = previousWindow.Add(time.Second)

	for i := 0; i < 5; i++ {
		curr, prev, _ := limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
		if curr != 0 {
			t.Errorf("unexpected curr = %v, expected %v", curr, 0)
		}
		if prev != 50 {
			t.Errorf("unexpected prev = %v, expected %v", prev, 50)
		}
		_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, 99)
	}

	// Time += 10
	currentWindow = currentWindow.Add(10 * time.Second)
	previousWindow = previousWindow.Add(10 * time.Second)

	for i := 0; i < 5; i++ {
		curr, prev, _ := limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
		if curr != 0 {
			t.Errorf("unexpected curr = %v, expected %v", curr, 0)
		}
		if prev != 0 {
			t.Errorf("unexpected prev = %v, expected %v", prev, 0)
		}
		_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, 99)
	}
}

func BenchmarkLocalCounter(b *testing.B) {
	limitCounter := &localCounter{
		latestWindow:     time.Now().UTC().Truncate(time.Second),
		latestCounters:   make(map[uint64]int),
		previousCounters: make(map[uint64]int),
		windowLength:     time.Second,
	}

	currentWindow := time.Now().UTC().Truncate(time.Second)
	previousWindow := currentWindow.Add(-time.Second)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := range []int{0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 3, 0, 0, 0, 0, 1, 0} {
			// Simulate time.
			currentWindow.Add(time.Duration(i) * time.Second)
			previousWindow.Add(time.Duration(i) * time.Second)

			wg := sync.WaitGroup{}
			wg.Add(1000)
			for i := 0; i < 1000; i++ {
				// Simulate concurrent requests with different rate-limit keys.
				go func(i int) {
					defer wg.Done()

					_, _, _ = limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
					_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, rand.Intn(100))
				}(i)
			}
			wg.Wait()
		}
	}
}
