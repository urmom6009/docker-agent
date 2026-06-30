package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker-agent/pkg/paths"
	"github.com/docker/docker-agent/pkg/remote"
)

const (
	DockerCatalogURL     = "https://desktop.docker.com/mcp/catalog/v3/catalog.yaml"
	catalogCacheFileName = "mcp_catalog.json"
	fetchTimeout         = 15 * time.Second

	// catalogJSON is the URL we actually fetch (JSON is ~3x faster to parse than YAML).
	catalogJSON = "https://desktop.docker.com/mcp/catalog/v3/catalog.json"
)

func RequiredEnvVars(ctx context.Context, serverName string) ([]Secret, error) {
	server, err := ServerSpec(ctx, serverName)
	if err != nil {
		return nil, err
	}

	// TODO(dga): until the MCP Gateway supports oauth with docker agent,
	// we ignore every secret listed on `remote` servers and assume
	// we can use oauth by connecting directly to the server's url.
	if server.Type == "remote" {
		return nil, nil
	}

	return server.Secrets, nil
}

func ServerSpec(ctx context.Context, serverName string) (Server, error) {
	catalog, err := loaderFrom(ctx).load(ctx)
	if err != nil {
		return Server{}, err
	}

	server, ok := catalog[serverName]
	if !ok {
		return Server{}, fmt.Errorf("MCP server %q not found in MCP catalog", serverName)
	}

	return server, nil
}

// ParseServerRef strips the optional "docker:" prefix from a server reference.
func ParseServerRef(ref string) string {
	return strings.TrimPrefix(ref, "docker:")
}

// cachedCatalog is the on-disk cache format.
type cachedCatalog struct {
	Catalog Catalog `json:"catalog"`
	ETag    string  `json:"etag,omitempty"`
}

// Loader fetches and memoizes the MCP catalog. A single Loader fetches the
// catalog at most once and reuses the memoized result for every later call,
// so callers should share one instance.
//
// The zero value is not usable; construct with [NewLoader] (production, fetches
// from the network) or [NewStaticLoader] (tests, serves a fixed catalog with no
// network access). A Loader is carried on the context via [WithLoader] and
// retrieved with [loaderFrom]; call sites that don't inject one transparently
// use the shared [defaultLoader].
type Loader struct {
	// fetch loads the catalog. Production uses fetchAndCache; tests inject a
	// stub that serves a fixed catalog without touching the network.
	fetch func(context.Context) (Catalog, error)

	once   sync.Once
	cached struct {
		catalog Catalog
		err     error
	}
}

// NewLoader returns a Loader that fetches the catalog from the network (with
// on-disk cache fallback) on first use and memoizes the result.
func NewLoader() *Loader {
	return &Loader{fetch: fetchAndCache}
}

// NewStaticLoader returns a Loader that always serves catalog without touching
// the network. It is the test seam that replaces the old global override: a
// test injects it via [WithLoader] so its goroutines never race a shared
// package-level variable.
func NewStaticLoader(catalog Catalog) *Loader {
	return &Loader{
		fetch: func(context.Context) (Catalog, error) { return catalog, nil },
	}
}

// load fetches and caches the catalog on the first call, reusing the memoized
// result afterwards. The first caller's context is honoured for tracing (and
// threads the fetch onto the program's context), but its cancellation is
// detached via WithoutCancel so a cancelled first caller cannot permanently
// cache a context-cancellation error for everyone else. On every subsequent
// call ctx is unused — the work has already run.
func (l *Loader) load(ctx context.Context) (Catalog, error) {
	// Default the fetcher so a zero-value Loader degrades to the production
	// fetch path instead of panicking inside once.Do (which would also wedge
	// the sync.Once into a permanently-done state).
	fetch := l.fetch
	if fetch == nil {
		fetch = fetchAndCache
	}
	l.once.Do(func() {
		l.cached.catalog, l.cached.err = fetch(context.WithoutCancel(ctx))
	})
	return l.cached.catalog, l.cached.err
}

// defaultLoader backs every call site that doesn't inject its own Loader via
// [WithLoader]. It preserves the historical behaviour of a single
// process-wide, fetched-once catalog.
var defaultLoader = NewLoader()

type loaderContextKey struct{}

// WithLoader returns a copy of ctx that carries loader, so calls to
// [ServerSpec] / [RequiredEnvVars] made with the returned context resolve
// against it instead of the shared [defaultLoader]. Tests use it together with
// [NewStaticLoader] to serve a fixed catalog without global state. A nil
// loader is ignored so callers fall back to the [defaultLoader].
func WithLoader(ctx context.Context, loader *Loader) context.Context {
	if loader == nil {
		return ctx
	}
	return context.WithValue(ctx, loaderContextKey{}, loader)
}

// loaderFrom returns the Loader carried by ctx, or the shared [defaultLoader]
// when none was injected.
func loaderFrom(ctx context.Context) *Loader {
	if l, ok := ctx.Value(loaderContextKey{}).(*Loader); ok && l != nil {
		return l
	}
	return defaultLoader
}

// fetchAndCache tries to fetch the catalog from the network (using ETag for
// conditional requests) and falls back to the disk cache on any failure.
func fetchAndCache(ctx context.Context) (Catalog, error) {
	cacheFile := cacheFilePath()
	cached := loadFromDisk(cacheFile)

	catalog, newETag, err := fetchFromNetwork(ctx, cached.ETag)
	if err != nil {
		slog.DebugContext(ctx, "Failed to fetch MCP catalog from network, using cache", "error", err)
		if cached.Catalog != nil {
			return cached.Catalog, nil
		}
		return nil, fmt.Errorf("fetching MCP catalog: %w (no cached copy available)", err)
	}

	// A nil catalog means 304 Not Modified — the cached copy is still valid.
	if catalog == nil {
		slog.DebugContext(ctx, "MCP catalog not modified (ETag match)")
		return cached.Catalog, nil
	}

	slog.DebugContext(ctx, "MCP catalog fetched from network")
	saveToDisk(cacheFile, catalog, newETag)

	return catalog, nil
}

func cacheFilePath() string {
	return filepath.Join(paths.GetCacheDir(), catalogCacheFileName)
}

func loadFromDisk(path string) cachedCatalog {
	data, err := os.ReadFile(path)
	if err != nil {
		return cachedCatalog{}
	}

	var cached cachedCatalog
	if err := json.Unmarshal(data, &cached); err != nil {
		return cachedCatalog{}
	}

	return cached
}

func saveToDisk(path string, catalog Catalog, etag string) {
	data, err := json.Marshal(cachedCatalog{Catalog: catalog, ETag: etag})
	if err != nil {
		slog.Warn("Failed to marshal MCP catalog cache", "error", err)
		return
	}

	dir := filepath.Dir(path)

	// Write to a temp file and rename so readers never see a partial file.
	// Try creating the temp file first; only create the directory if needed.
	tmp, err := os.CreateTemp(dir, ".mcp_catalog_*.tmp")
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("Failed to create MCP catalog temp file", "error", err)
			return
		}
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil { //nolint:gosec // shared with other docker MCP gateway processes
			slog.Warn("Failed to create MCP catalog cache directory", "error", mkErr)
			return
		}
		tmp, err = os.CreateTemp(dir, ".mcp_catalog_*.tmp")
		if err != nil {
			slog.Warn("Failed to create MCP catalog temp file", "error", err)
			return
		}
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		slog.Warn("Failed to write MCP catalog temp file", "error", err)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		slog.Warn("Failed to close MCP catalog temp file", "error", err)
		return
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		slog.Warn("Failed to rename MCP catalog cache file", "error", err)
	}
}

// fetchFromNetwork fetches the catalog, using the ETag for conditional requests.
// It returns (nil, "", nil) when the server responds with 304 Not Modified.
func fetchFromNetwork(ctx context.Context, etag string) (Catalog, string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogJSON, http.NoBody)
	if err != nil {
		return nil, "", err
	}

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	catalogClient := &http.Client{Transport: remote.NewTransport(ctx)}
	resp, err := catalogClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, "", nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status fetching MCP catalog: %s", resp.Status)
	}

	var top topLevel
	if err := json.NewDecoder(resp.Body).Decode(&top); err != nil {
		return nil, "", err
	}

	return top.Catalog, resp.Header.Get("ETag"), nil
}
