package monobank

import (
	"container/list"
	"sync"
)

// Deduper remembers transaction IDs seen recently so callers can answer
// "did we already process this event?". Mono retries failed deliveries
// after 60s and 600s; a deduper of capacity ≥ a few hundred is plenty for
// most workloads.
//
// The default LRU implementation ([NewMemoryDeduper]) is safe for
// concurrent use. Plug in your own implementation (Redis, SQLite, …) by
// satisfying the [Deduper] interface — for example to share state across
// instances.
type Deduper interface {
	// Seen reports whether id has been observed before. If the id is new
	// it is recorded as seen and Seen returns false.
	Seen(id string) bool
}

// NewMemoryDeduper returns an in-memory LRU [Deduper] of the given capacity.
// Capacity ≤ 0 falls back to 1024.
func NewMemoryDeduper(capacity int) *MemoryDeduper {
	if capacity <= 0 {
		capacity = 1024
	}
	return &MemoryDeduper{
		capacity: capacity,
		order:    list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

// MemoryDeduper is a fixed-size LRU set of strings, safe for concurrent use.
type MemoryDeduper struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	index    map[string]*list.Element
}

// Seen records id and returns whether it had already been recorded.
func (d *MemoryDeduper) Seen(id string) bool {
	if id == "" {
		// Empty id can't be deduped (no stable key); treat as new every time
		// rather than collapse every empty-id event into one.
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if el, ok := d.index[id]; ok {
		d.order.MoveToFront(el)
		return true
	}
	if d.order.Len() == d.capacity {
		oldest := d.order.Back()
		if oldest != nil {
			delete(d.index, oldest.Value.(string))
			d.order.Remove(oldest)
		}
	}
	d.index[id] = d.order.PushFront(id)
	return false
}

// Len returns the number of currently-tracked ids; useful for diagnostics.
func (d *MemoryDeduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.order.Len()
}
