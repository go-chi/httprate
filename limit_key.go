package httprate

import (
	"strconv"
	"time"

	"github.com/zeebo/xxh3"
)

// LimitCounterKey computes a hash key for the given key and window.
func LimitCounterKey(key string, window time.Time) uint64 {
	h := xxh3.New()
	_, _ = h.WriteString(key)
	_, _ = h.WriteString(strconv.FormatInt(window.Unix(), 10))
	return h.Sum64()
}
