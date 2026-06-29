package scrollbar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateThumbPosition(t *testing.T) {
	t.Parallel()
	sb := New()
	sb.SetDimensions(10, 100)

	// At top
	sb.SetScrollOffset(0)
	top, height := sb.calculateThumbPosition()
	assert.Equal(t, 0, top)
	assert.Positive(t, height)

	// At middle
	sb.SetScrollOffset(45)
	top, height = sb.calculateThumbPosition()
	assert.Positive(t, top)
	assert.Less(t, top+height, sb.height)

	// At bottom
	sb.SetScrollOffset(90)
	top, height = sb.calculateThumbPosition()
	assert.Equal(t, sb.height-height, top)
}

func TestScrollMethods(t *testing.T) {
	t.Parallel()
	sb := New()
	sb.SetDimensions(10, 100)

	t.Run("ScrollDown", func(t *testing.T) {
		sb.SetScrollOffset(0)
		sb.ScrollDown()
		assert.Equal(t, 1, sb.scrollOffset)
	})

	t.Run("ScrollUp", func(t *testing.T) {
		sb.SetScrollOffset(10)
		sb.ScrollUp()
		assert.Equal(t, 9, sb.scrollOffset)
	})

	t.Run("PageDown", func(t *testing.T) {
		sb.SetScrollOffset(0)
		sb.PageDown()
		assert.Equal(t, 10, sb.scrollOffset)
	})

	t.Run("PageUp", func(t *testing.T) {
		sb.SetScrollOffset(20)
		sb.PageUp()
		assert.Equal(t, 10, sb.scrollOffset)
	})
}
