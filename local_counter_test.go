package httprate

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func BenchmarkLocalCounter(b *testing.B) {
	limitCounter := &localCounter{
		counters:     make(map[uint64]*count),
		windowLength: time.Second,
	}

	t := time.Now().UTC()
	currentWindow := t.Truncate(time.Second)
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
