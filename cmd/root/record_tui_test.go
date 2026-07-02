package root

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteUniqueFile_NeverOverwrites(t *testing.T) {
	base := filepath.Join(t.TempDir(), "rec")

	existing := base + "_test.go"
	require.NoError(t, os.WriteFile(existing, []byte("precious"), 0o600))

	path, err := writeUniqueFile(base, []byte("generated"))
	require.NoError(t, err)

	assert.Equal(t, base+"_2_test.go", path)
	got, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "precious", string(got), "pre-existing file must not be clobbered")
}

func TestWriteUniqueFile_WritesFirstCandidate(t *testing.T) {
	base := filepath.Join(t.TempDir(), "rec")

	path, err := writeUniqueFile(base, []byte("generated"))
	require.NoError(t, err)

	assert.Equal(t, base+"_test.go", path)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "generated", string(got))
}
