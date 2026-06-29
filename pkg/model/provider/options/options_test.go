package options

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sentinelTransport is a minimal http.RoundTripper used only for identity checks.
type sentinelTransport struct{ base http.RoundTripper }

func (s *sentinelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return s.base.RoundTrip(req)
}

func TestWithHTTPTransportWrapper_SetAndGet(t *testing.T) {
	t.Parallel()
	var called bool
	wrapFn := func(base http.RoundTripper) http.RoundTripper {
		called = true
		return &sentinelTransport{base: base}
	}

	var opts ModelOptions
	WithHTTPTransportWrapper(wrapFn)(&opts)

	got := opts.TransportWrapper()
	require.NotNil(t, got)

	// Verify invoking the returned wrapper marks called=true and returns a non-nil transport.
	result := got(http.DefaultTransport)
	assert.True(t, called)
	assert.NotNil(t, result)
}

func TestTransportWrapper_NilByDefault(t *testing.T) {
	t.Parallel()
	var opts ModelOptions
	assert.Nil(t, opts.TransportWrapper())
}

func TestFromModelOptions_RoundTripsTransportWrapper(t *testing.T) {
	t.Parallel()
	var wrapperInvoked bool
	wrapFn := func(base http.RoundTripper) http.RoundTripper {
		wrapperInvoked = true
		return &sentinelTransport{base: base}
	}

	var src ModelOptions
	WithHTTPTransportWrapper(wrapFn)(&src)

	opts := FromModelOptions(src)
	require.NotEmpty(t, opts)

	var dst ModelOptions
	for _, o := range opts {
		o(&dst)
	}

	got := dst.TransportWrapper()
	require.NotNil(t, got)

	result := got(http.DefaultTransport)
	assert.True(t, wrapperInvoked)
	assert.NotNil(t, result)
}

func TestFromModelOptions_NilWrapperNotIncluded(t *testing.T) {
	t.Parallel()
	// A ModelOptions with no transport wrapper should not add a
	// WithHTTPTransportWrapper opt, so TransportWrapper() stays nil.
	var src ModelOptions
	opts := FromModelOptions(src)

	var dst ModelOptions
	for _, o := range opts {
		o(&dst)
	}

	assert.Nil(t, dst.TransportWrapper())
}
