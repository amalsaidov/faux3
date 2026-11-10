package store

import (
	"sort"
	"sync"
)

// index is the per-bucket in-memory key index: a sorted slice for listing
// plus a map for lookups. Insert and delete are O(n) slice moves, which is
// fine for a development tool.
type index struct {
	mu    sync.RWMutex
	keys  []string
	metas map[string]*Meta
}

func newIndex() *index { return &index{metas: map[string]*Meta{}} }

func (ix *index) get(key string) (Meta, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	m, ok := ix.metas[key]
	if !ok {
		return Meta{}, false
	}
	return *m, true
}

func (ix *index) put(m *Meta) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if _, ok := ix.metas[m.Key]; !ok {
		i := sort.SearchStrings(ix.keys, m.Key)
		ix.keys = append(ix.keys, "")
		copy(ix.keys[i+1:], ix.keys[i:])
		ix.keys[i] = m.Key
	}
	ix.metas[m.Key] = m
}

func (ix *index) remove(key string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if _, ok := ix.metas[key]; !ok {
		return
	}
	delete(ix.metas, key)
	i := sort.SearchStrings(ix.keys, key)
	ix.keys = append(ix.keys[:i], ix.keys[i+1:]...)
}

func (ix *index) len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return len(ix.keys)
}

// metaLocked returns the stored meta for key. The caller must hold ix.mu;
// get must not be used then, since a recursive RLock can deadlock behind a
// waiting writer.
func (ix *index) metaLocked(key string) Meta { return *ix.metas[key] }
