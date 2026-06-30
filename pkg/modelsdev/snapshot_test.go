package modelsdev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedSnapshotParses ensures the snapshot baked into the binary is
// valid JSON and carries a non-trivial catalog. A broken snapshot would
// silently degrade every offline lookup, so we guard it at build time.
func TestEmbeddedSnapshotParses(t *testing.T) {
	t.Parallel()

	db := embeddedSnapshot()
	require.NotNil(t, db)
	assert.NotEmpty(t, db.Providers, "embedded snapshot must contain providers")

	// Spot-check a provider/model we always expect to be present.
	openai, ok := db.Providers["openai"]
	require.True(t, ok, "embedded snapshot must contain the openai provider")
	assert.NotEmpty(t, openai.Models, "openai provider must list models")
}

// TestSnapshotDateParses verifies the embedded snapshot date is a valid,
// non-zero RFC3339 timestamp. This always runs: a malformed date is a build
// artefact bug, not a function of wall-clock time.
func TestSnapshotDateParses(t *testing.T) {
	t.Parallel()

	require.False(t, SnapshotDate().IsZero(),
		"snapshot_date.txt must contain a valid RFC3339 date")
}

// TestSnapshotDateIsFresh fails when the embedded snapshot is older than
// maxAge. It is the freshness gate that reminds maintainers to refresh the
// snapshot when the scheduled refresh workflow stops working.
//
// It is opt-in (gated on CHECK_MODELS_SNAPSHOT_FRESHNESS) and skipped by
// default. A snapshot ages purely with wall-clock time, so wiring this into
// the normal `go test ./...` gate would eventually break unrelated work on
// every fork and stale branch — a flaky, time-dependent failure. The actual
// freshness mechanism is the weekly update-models workflow; this assertion is
// a backstop the maintainer repo can opt into.
func TestSnapshotDateIsFresh(t *testing.T) {
	t.Parallel()

	if os.Getenv("CHECK_MODELS_SNAPSHOT_FRESHNESS") == "" {
		t.Skip("set CHECK_MODELS_SNAPSHOT_FRESHNESS=1 to enforce snapshot freshness")
	}

	const maxAge = 90 * 24 * time.Hour

	date := SnapshotDate()
	require.False(t, date.IsZero(), "snapshot_date.txt must contain a valid RFC3339 date")

	age := time.Since(date)
	assert.Less(t, age, maxAge,
		"embedded models.dev snapshot is %s old; refresh it with `go generate ./pkg/modelsdev/...`", age)
}

// TestColdCacheFallsBackToSnapshot verifies the new behaviour: when the cache
// is cold and the network is unreachable, a known-provider lookup resolves
// against the embedded snapshot instead of erroring out.
func TestColdCacheFallsBackToSnapshot(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models_dev.json")
	fetched, fetch := trackingFetcher()
	store, err := NewStore(WithCache(cacheFile), WithFetcher(fetch))
	require.NoError(t, err)

	// gpt-4o is present in the embedded snapshot; the fetch fails, so the
	// resolution must come from the snapshot.
	m, err := store.GetModel(t.Context(), NewID("openai", "gpt-4o"))
	require.NoError(t, err)
	assert.True(t, *fetched, "a cold cache must still attempt a fetch first")
	assert.NotEmpty(t, m.Name)
}

// TestSnapshotFallbackIsNotMemoized guards against pinning the embedded
// fallback for the Store's lifetime: a transient fetch failure must degrade to
// the snapshot, but a later lookup (once the network recovers) must retry the
// fetch and serve the fresh catalog rather than the stale build-time snapshot.
func TestSnapshotFallbackIsNotMemoized(t *testing.T) {
	t.Parallel()

	cacheFile := filepath.Join(t.TempDir(), "models_dev.json")

	// First lookup: the network is down, so the fetch fails and we fall back
	// to the embedded snapshot without memoizing it.
	var firstFetched, secondFetched bool
	fetch := func(context.Context, string) (*Database, string, error) {
		if !firstFetched {
			firstFetched = true
			return nil, "", errors.New("fetch from API: network unreachable")
		}
		secondFetched = true
		return &Database{Providers: map[string]Provider{
			"openai": {Models: map[string]Model{
				"fresh-model": {Name: "Freshly Fetched", Limit: Limit{Context: 12345}},
			}},
		}}, "etag-1", nil
	}
	store, err := NewStore(WithCache(cacheFile), WithFetcher(fetch))
	require.NoError(t, err)

	_, err = store.GetModel(t.Context(), NewID("openai", "gpt-4o"))
	require.NoError(t, err)
	assert.True(t, firstFetched)

	// Network recovers: a second lookup must retry the fetch (the fallback was
	// not pinned) and serve the freshly fetched catalog.
	m, err := store.GetModel(t.Context(), NewID("openai", "fresh-model"))
	require.NoError(t, err)
	assert.True(t, secondFetched, "a fetch failure must not be memoized; the next lookup must retry")
	assert.Equal(t, 12345, m.Limit.Context, "the recovered fetch must serve fresh data, not the snapshot")
}
