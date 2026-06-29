package concurrent

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuffer_Write(t *testing.T) {
	t.Parallel()
	var b Buffer

	n, err := b.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = b.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	assert.Equal(t, "hello world", b.String())
}

func TestBuffer_Bytes(t *testing.T) {
	t.Parallel()
	var b Buffer
	_, _ = b.Write([]byte("hello"))

	got := b.Bytes()
	assert.Equal(t, []byte("hello"), got)

	// Mutating the returned slice must not affect the buffer.
	got[0] = 'H'
	assert.Equal(t, "hello", b.String())
}

func TestBuffer_Len(t *testing.T) {
	t.Parallel()
	var b Buffer
	assert.Equal(t, 0, b.Len())

	_, _ = b.Write([]byte("abc"))
	assert.Equal(t, 3, b.Len())

	_, _ = b.Write([]byte("de"))
	assert.Equal(t, 5, b.Len())
}

func TestBuffer_Reset(t *testing.T) {
	t.Parallel()
	var b Buffer
	_, _ = b.Write([]byte("hello"))

	b.Reset()
	assert.Equal(t, 0, b.Len())
	assert.Empty(t, b.String())
}

func TestBuffer_Drain(t *testing.T) {
	t.Parallel()
	var b Buffer
	_, _ = b.Write([]byte("hello"))

	got := b.Drain()
	assert.Equal(t, "hello", got)
	assert.Equal(t, 0, b.Len())
	assert.Empty(t, b.String())
}

func TestBuffer_Concurrent(t *testing.T) {
	t.Parallel()
	var b Buffer
	var wg sync.WaitGroup

	const writers = 100
	for i := range writers {
		wg.Go(func() {
			_, _ = b.Write(fmt.Appendf(nil, "%03d", i))
		})
	}

	// Concurrent readers should not race with writers.
	for range 50 {
		wg.Go(func() {
			_ = b.String()
			_ = b.Len()
			_ = b.Bytes()
		})
	}

	wg.Wait()
	assert.Equal(t, writers*3, b.Len())
}
