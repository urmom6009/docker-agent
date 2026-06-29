package lrucache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRU_GetReturnsStoredValues(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	for k, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		got, ok := c.Get(k)
		assert.True(t, ok, "expected %q to be present", k)
		assert.Equal(t, want, got)
	}
}

func TestLRU_GetMissReturnsZero(t *testing.T) {
	t.Parallel()
	c := New[string, int](2)

	v, ok := c.Get("missing")
	assert.False(t, ok)
	assert.Zero(t, v)
}

func TestLRU_PutEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	_, ok := c.Get("a")
	assert.False(t, ok, "'a' should have been evicted")
	assertGet(t, c, "b", 2)
	assertGet(t, c, "c", 3)
}

func TestLRU_GetPromotesEntry(t *testing.T) {
	t.Parallel()
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a") // promote 'a' so 'b' becomes LRU
	c.Put("c", 3)

	_, ok := c.Get("b")
	assert.False(t, ok, "'b' should have been evicted after 'a' was promoted")
	assertGet(t, c, "a", 1)
}

func TestLRU_PutOnExistingKeyUpdatesAndPromotes(t *testing.T) {
	t.Parallel()
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 10) // update + promote
	c.Put("c", 3)  // should evict 'b'

	assertGet(t, c, "a", 10)
	_, ok := c.Get("b")
	assert.False(t, ok, "'b' should have been evicted")
	assertGet(t, c, "c", 3)
	assert.Equal(t, 2, c.Len())
}

func TestLRU_Delete(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)

	c.Delete("a")
	_, ok := c.Get("a")
	assert.False(t, ok)
	assert.Equal(t, 1, c.Len())

	c.Delete("missing") // no-op on absent key
	assert.Equal(t, 1, c.Len())
}

func TestLRU_Clear(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)

	c.Clear()
	assert.Equal(t, 0, c.Len())
	_, ok := c.Get("a")
	assert.False(t, ok)

	// Still usable after clear.
	c.Put("c", 3)
	assertGet(t, c, "c", 3)
}

func TestLRU_NonPositiveSizeIsClampedToOne(t *testing.T) {
	t.Parallel()
	c := New[string, int](0)
	c.Put("a", 1)
	c.Put("b", 2)

	assert.Equal(t, 1, c.Len())
	_, ok := c.Get("a")
	assert.False(t, ok, "'a' should have been evicted at capacity 1")
	assertGet(t, c, "b", 2)
}

func TestLRU_RangeVisitsAllEntriesInLRUOrder(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	c.Get("a") // promote 'a' so order is: b (lru), c, a (mru)

	var keys []string
	c.Range(func(k string, _ int) bool {
		keys = append(keys, k)
		return true
	})
	assert.Equal(t, []string{"b", "c", "a"}, keys)
}

func TestLRU_RangeStopsWhenFnReturnsFalse(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	visited := 0
	c.Range(func(string, int) bool {
		visited++
		return false
	})
	assert.Equal(t, 1, visited)
}

func TestLRU_RangeOnEmptyCacheIsNoOp(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)

	visited := 0
	c.Range(func(string, int) bool {
		visited++
		return true
	})
	assert.Equal(t, 0, visited)
}

func TestLRU_LenTracksEntryCount(t *testing.T) {
	t.Parallel()
	c := New[string, int](3)
	assert.Equal(t, 0, c.Len())

	c.Put("a", 1)
	c.Put("b", 2)
	assert.Equal(t, 2, c.Len())

	c.Put("a", 10) // update, no growth
	assert.Equal(t, 2, c.Len())

	c.Put("c", 3)
	c.Put("d", 4) // evicts oldest, len stays at maxSize
	assert.Equal(t, 3, c.Len())
}

func assertGet[K comparable, V any](t *testing.T, c *LRU[K, V], key K, want V) {
	t.Helper()
	got, ok := c.Get(key)
	assert.True(t, ok, "expected key to be present")
	assert.Equal(t, want, got)
}
