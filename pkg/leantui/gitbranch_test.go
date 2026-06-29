package leantui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadHeadBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	head := filepath.Join(dir, "HEAD")
	require.NoError(t, os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o644))
	assert.Equal(t, "main", readHead(head))
}

func TestReadHeadSlashedBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	head := filepath.Join(dir, "HEAD")
	require.NoError(t, os.WriteFile(head, []byte("ref: refs/heads/feature/login\n"), 0o644))
	assert.Equal(t, "feature/login", readHead(head))
}

func TestReadHeadDetached(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	head := filepath.Join(dir, "HEAD")
	require.NoError(t, os.WriteFile(head, []byte("0123456789abcdef\n"), 0o644))
	assert.Equal(t, "0123456", readHead(head))
}

func TestGitBranchWalksUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/dev\n"), 0o644))

	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	assert.Equal(t, "dev", gitBranch(nested))
}

func TestGitBranchNoRepo(t *testing.T) {
	t.Parallel()
	assert.Empty(t, gitBranch(t.TempDir()))
}

func TestShortenPath(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, "~", shortenPath(home))
	assert.Equal(t, filepath.Join("~", "x"), shortenPath(filepath.Join(home, "x")))
	assert.Equal(t, "/etc", shortenPath("/etc"))
}
