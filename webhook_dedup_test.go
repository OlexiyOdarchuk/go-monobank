package monobank

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryDeduper_basic(t *testing.T) {
	d := NewMemoryDeduper(3)

	assert.False(t, d.Seen("a"))
	assert.True(t, d.Seen("a"), "second visit must return true")
	assert.False(t, d.Seen("b"))
	assert.False(t, d.Seen("c"))
	assert.Equal(t, 3, d.Len())

	// d now: most recent → least: c, b, a
	// Inserting "d" evicts "a"
	assert.False(t, d.Seen("d"))
	assert.False(t, d.Seen("a"), "evicted id must be treated as new again")
	assert.Equal(t, 3, d.Len())
}

func TestMemoryDeduper_emptyIdAlwaysNew(t *testing.T) {
	d := NewMemoryDeduper(8)
	assert.False(t, d.Seen(""))
	assert.False(t, d.Seen(""), "empty id must not collapse into a single slot")
}

func TestMemoryDeduper_LRUMovesAccessedToFront(t *testing.T) {
	d := NewMemoryDeduper(3)

	d.Seen("a")
	d.Seen("b")
	d.Seen("c")
	// Touch "a" — should now be most-recent.
	assert.True(t, d.Seen("a"))
	// Inserting a new id evicts "b" (now LRU), not "a".
	d.Seen("d")
	assert.True(t, d.Seen("a"))  // still present
	assert.False(t, d.Seen("b")) // evicted, fresh visit
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
				_ = d.Seen(id)
			}
		}(w)
	}
	wg.Wait()

	// Just assert we didn't blow past capacity and didn't panic.
	assert.LessOrEqual(t, d.Len(), 1024)
}

func TestMemoryDeduper_zeroCapacityFallback(t *testing.T) {
	d := NewMemoryDeduper(0)
	assert.False(t, d.Seen("x"))
	assert.True(t, d.Seen("x"))
}
