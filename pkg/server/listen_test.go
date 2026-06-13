package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListen_FD(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig
	orig, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer orig.Close()

	file, err := orig.(*net.TCPListener).File()
	require.NoError(t, err)
	defer file.Close()

	ln, err := Listen(t.Context(), fmt.Sprintf("fd://%d", file.Fd()))
	require.NoError(t, err)
	defer ln.Close()

	assert.NotNil(t, ln.Addr())
}

func TestListen_FD_InvalidNumber(t *testing.T) {
	t.Parallel()

	_, err := Listen(t.Context(), "fd://notanumber")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid file descriptor")
}

func TestListen_FD_NegativeNumber(t *testing.T) {
	t.Parallel()

	_, err := Listen(t.Context(), "fd://-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be > 2")
}

func TestListen_FD_ReservedDescriptors(t *testing.T) {
	t.Parallel()

	for _, fd := range []string{"0", "1", "2"} {
		_, err := Listen(t.Context(), "fd://"+fd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be > 2")
	}
}

func TestListen_FD_InvalidDescriptor(t *testing.T) {
	t.Parallel()

	// Use a very high fd number that's unlikely to exist
	_, err := Listen(t.Context(), "fd://999999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file descriptor 999999")
}

// TestListen_TCP_IPv4 verifies that the default TCP listener binds to an
// IPv4 loopback address. Regression test for the listener being hard-coded
// to "tcp4" without that being the documented intent.
func TestListen_TCP_IPv4(t *testing.T) {
	t.Parallel()

	ln, err := Listen(t.Context(), "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	assert.NotNil(t, tcpAddr.IP.To4())
}

// TestListen_TCP_IPv6 verifies that an IPv6 loopback bind succeeds. The
// listener used to force "tcp4" which made this fail with
// "address ::1: non-IPv4 address" on dual-stack hosts.
func TestListen_TCP_IPv6(t *testing.T) {
	t.Parallel()

	// Probe whether the host actually has IPv6 before asserting.
	var probeLC net.ListenConfig
	probe, probeErr := probeLC.Listen(t.Context(), "tcp6", "[::1]:0")
	if probeErr != nil {
		t.Skipf("host does not support IPv6 loopback: %v", probeErr)
	}
	_ = probe.Close()

	ln, err := Listen(t.Context(), "[::1]:0")
	require.NoError(t, err)
	defer ln.Close()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	assert.True(t, tcpAddr.IP.IsLoopback())
	assert.Nil(t, tcpAddr.IP.To4(), "expected an IPv6-only address")
}

// shortTempDir returns a temp dir with a short path so unix socket paths
// created under it stay within the platform limit (macOS caps sun_path at
// ~104 bytes, which t.TempDir()'s long, test-name-derived paths can exceed).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ls") //nolint:forbidigo,usetesting // need a short path for the unix sun_path limit (~104 bytes); t.TempDir() embeds the long test name and overflows it
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestListen_Unix verifies that --listen accepts a unix:// socket path,
// creates the parent directory and a usable socket, and serves HTTP over it.
// The board uses a deterministic per-session socket path to avoid TCP port
// allocation and collisions, so this is the path it relies on.
func TestListen_Unix(t *testing.T) {
	t.Parallel()

	// Nest under a not-yet-existing dir to exercise MkdirAll.
	sockPath := filepath.Join(shortTempDir(t), "run", "a.sock")

	ln, err := Listen(t.Context(), "unix://"+sockPath)
	require.NoError(t, err)
	defer ln.Close()

	assert.Equal(t, "unix", ln.Addr().Network())
	_, statErr := os.Stat(sockPath)
	require.NoError(t, statErr, "socket file should exist")

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	srv := &http.Server{Handler: mux}
	defer srv.Close()
	go func() { _ = srv.Serve(ln) }()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sockPath)
			},
		},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/ping", http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(body))
}

// TestListen_Unix_ReplacesStaleSocket verifies that a leftover socket file
// from a previous run is removed so the bind succeeds (rather than failing
// with "address already in use").
func TestListen_Unix_ReplacesStaleSocket(t *testing.T) {
	t.Parallel()

	sockPath := filepath.Join(shortTempDir(t), "a.sock")

	ln1, err := Listen(t.Context(), "unix://"+sockPath)
	require.NoError(t, err)
	require.NoError(t, ln1.Close())

	// The socket file lingers after close; a fresh Listen must reclaim it.
	ln2, err := Listen(t.Context(), "unix://"+sockPath)
	require.NoError(t, err)
	defer ln2.Close()
	assert.Equal(t, "unix", ln2.Addr().Network())
}
