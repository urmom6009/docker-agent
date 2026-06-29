package environment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMultiProviderNone(t *testing.T) {
	t.Parallel()
	provider := NewMultiProvider()
	value, found := provider.Get(t.Context(), "TEST1")

	assert.Empty(t, value)
	assert.False(t, found)
}

func TestMultiProviderDelegate(t *testing.T) {
	t.Parallel()
	provider := NewMultiProvider(&alwaysFound{}, &neverFound{})
	value, found := provider.Get(t.Context(), "TEST2")

	assert.Equal(t, "FOUND", value)
	assert.True(t, found)
}

func TestMultiProviderTryInOrder(t *testing.T) {
	t.Parallel()
	provider := NewMultiProvider(&neverFound{}, &alwaysFound{})
	value, found := provider.Get(t.Context(), "TEST3")

	assert.Equal(t, "FOUND", value)
	assert.True(t, found)
}

func TestMultiProviderEmptyValue(t *testing.T) {
	t.Parallel()
	firstProvider := NewEnvListProvider([]string{"MY_VAR="})
	secondProvider := NewEnvListProvider([]string{"MY_VAR=fallback"})

	provider := NewMultiProvider(firstProvider, secondProvider)
	value, found := provider.Get(t.Context(), "MY_VAR")

	assert.True(t, found)
	assert.Empty(t, value)
}

type neverFound struct{}

func (p *neverFound) Get(context.Context, string) (string, bool) {
	return "", false
}

type alwaysFound struct{}

func (p *alwaysFound) Get(context.Context, string) (string, bool) {
	return "FOUND", true
}
