package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFallbackModels(t *testing.T) {
	t.Parallel()
	t.Run("absent", func(t *testing.T) {
		assert.Nil(t, fallbackModels(nil))
		assert.Nil(t, fallbackModels(map[string]any{"top_k": 40}))
	})

	t.Run("empty list", func(t *testing.T) {
		assert.Nil(t, fallbackModels(map[string]any{"fallbacks": []any{}}))
	})

	t.Run("invalid type", func(t *testing.T) {
		assert.Nil(t, fallbackModels(map[string]any{"fallbacks": "claude-opus-4-8"}))
	})

	t.Run("configured", func(t *testing.T) {
		assert.Equal(t,
			[]string{"claude-opus-4-8", "claude-sonnet-4-6"},
			fallbackModels(map[string]any{"fallbacks": []any{"claude-opus-4-8", "claude-sonnet-4-6"}}))
	})
}
