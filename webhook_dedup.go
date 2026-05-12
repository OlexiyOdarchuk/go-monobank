package monobank

import (
	"container/list"
	"sync"
)

// Deduper remembers transaction IDs the handler has already processed
// successfully so it can short-circuit retries from mono.
//
// Mono retries failed deliveries after 60s and 600s. The handler calls
// Has(id) before invoking OnEvent and Add(id) only after OnEvent succeeds
// — that way a transient OnEvent failure (which produces HTTP 500) does
// not poison the deduper and prevent the next retry from being processed.
//
// The default LRU implementation ([NewMemoryDeduper]) is safe for
// concurrent use. Plug in your own (Redis, SQLite, etc.) by satisfying
// the interface — useful for sharing dedup state across replicas.
type Deduper interface {
	// Has reports whether id has been recorded by a previous Add.
	Has(id string) bool
	// Add records id as processed. It is safe to call Add for the same
	// id more than once.
	Add(id string)
}

// NewMemoryDeduper returns an in-memory LRU Deduper of the given capacity.
// Capacity <= 0 falls back to 1024.
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

// Has reports whether id is currently tracked.
func (d *MemoryDeduper) Has(id string) bool {
	if id == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.index[id]
	if ok {
		// Refresh recency; if a caller is checking the same id repeatedly
		// they're "using" it.
		d.order.MoveToFront(d.index[id])
	}
	return ok
}

// Add records id as seen. No-op for empty ids.
func (d *MemoryDeduper) Add(id string) {
	if id == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if el, ok := d.index[id]; ok {
		d.order.MoveToFront(el)
		return
	}
	if d.order.Len() == d.capacity {
		if oldest := d.order.Back(); oldest != nil {
			delete(d.index, oldest.Value.(string))
			d.order.Remove(oldest)
		}
	}
	d.index[id] = d.order.PushFront(id)
}

// Len returns the number of currently-tracked ids; useful for diagnostics.
func (d *MemoryDeduper) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.order.Len()
}
