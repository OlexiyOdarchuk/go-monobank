package monobank

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryDeduper_basic(t *testing.T) {
	d := NewMemoryDeduper(3)

	assert.False(t, d.Has("a"))
	d.Add("a")
	assert.True(t, d.Has("a"))

	d.Add("b")
	d.Add("c")
	assert.Equal(t, 3, d.Len())

	// d now: most recent → least: c, b, a
	d.Add("e") // evicts "a"
	assert.False(t, d.Has("a"), "evicted id must be reported as new")
	assert.True(t, d.Has("b"))
	assert.True(t, d.Has("c"))
	assert.True(t, d.Has("e"))
	assert.Equal(t, 3, d.Len())
}

func TestMemoryDeduper_emptyIdNoop(t *testing.T) {
	d := NewMemoryDeduper(8)
	d.Add("")
	assert.False(t, d.Has(""), "empty id is ignored")
	assert.Equal(t, 0, d.Len())
}

func TestMemoryDeduper_AddIsIdempotent(t *testing.T) {
	d := NewMemoryDeduper(3)
	d.Add("a")
	d.Add("a")
	d.Add("a")
	assert.Equal(t, 1, d.Len())
}

func TestMemoryDeduper_LRUEvictsLeastRecentlyUsed(t *testing.T) {
	d := NewMemoryDeduper(3)
	d.Add("a")
	d.Add("b")
	d.Add("c")

	// Touch "a" — should refresh its recency.
	assert.True(t, d.Has("a"))
	d.Add("d") // evicts "b" (now LRU), not "a"
	assert.True(t, d.Has("a"))
	assert.False(t, d.Has("b"))
	assert.True(t, d.Has("c"))
	assert.True(t, d.Has("d"))
}

func TestMemoryDeduper_concurrent(t *testing.T) {
	d := NewMemoryDeduper(1024)

	const workers = 16
	const opsPerWorker = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				id := strconv.Itoa((w * opsPerWorker) + i)
				_ = d.Has(id)
				d.Add(id)
			}
		}(w)
	}
	wg.Wait()

	assert.LessOrEqual(t, d.Len(), 1024)
}

func TestMemoryDeduper_zeroCapacityFallback(t *testing.T) {
	d := NewMemoryDeduper(0)
	d.Add("x")
	assert.True(t, d.Has("x"))
}
