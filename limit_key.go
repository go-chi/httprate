package httprate

import (
	"fmt"
	"time"

	"github.com/zeebo/xxh3"
)

func LimitCounterKey(key string, window time.Time) uint64 {
	h := xxh3.New()
	h.WriteString(key)
	h.WriteString(fmt.Sprintf("%d", window.Unix()))
	return h.Sum64()
}
