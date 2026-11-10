package store

import (
	"hash/fnv"
	"sync"
)

const stripeCount = 256

// stripes is a fixed set of key locks selected by hashing bucket and key.
type stripes [stripeCount]sync.RWMutex

func (s *stripes) lock(bucket, key string) *sync.RWMutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(bucket))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	return &s[h.Sum32()%stripeCount]
}
