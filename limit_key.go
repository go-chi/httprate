package httprate

import (
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"
)

func LimitCounterKey(key string, window time.Time) uint64 {
	h := xxhash.New()
	h.WriteString(key)
	h.WriteString(fmt.Sprintf("%d", window.Unix()))
	return h.Sum64()
}
