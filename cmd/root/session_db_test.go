package root

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/paths"
)

// TestDefaultSessionDB checks the session DB path is resolved lazily from the
// data dir so --data-dir and SetRoot are honoured (regression: the old flag
// default baked ~/.cagent/session.db at registration time, splitting sessions).
func TestDefaultSessionDB(t *testing.T) {
	// Mutates the process-global data-dir override; cannot run in parallel.
	dataDir := t.TempDir()
	paths.SetDataDir(dataDir)
	t.Cleanup(func() { paths.SetDataDir("") })

	assert.Equal(t, filepath.Join(dataDir, "session.db"), defaultSessionDB(""),
		"empty value must resolve to <data-dir>/session.db after an override")

	assert.Equal(t, "/explicit/path.db", defaultSessionDB("/explicit/path.db"),
		"an explicit --session-db value must be returned unchanged")
}
