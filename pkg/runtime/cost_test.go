package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/chat"
)

// TestUsageHasTokens covers the helper that suppresses the missing-price
// warning for empty/no-op turns. The per-message cost arithmetic and its
// nil/unpriced branches are exercised by TestComputeMessageCost in
// after_llm_call_test.go, which shares the same computeMessageCost source.
func TestUsageHasTokens(t *testing.T) {
	t.Parallel()
	assert.False(t, usageHasTokens(nil), "nil usage has no tokens")
	assert.False(t, usageHasTokens(&chat.Usage{}), "zero usage has no tokens")
	assert.True(t, usageHasTokens(&chat.Usage{InputTokens: 1}))
	assert.True(t, usageHasTokens(&chat.Usage{OutputTokens: 1}))
	assert.True(t, usageHasTokens(&chat.Usage{CachedInputTokens: 1}))
	assert.True(t, usageHasTokens(&chat.Usage{CacheWriteTokens: 1}))
}
