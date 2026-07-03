package gitbranch

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

func TestCurrentWalksUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/dev\n"), 0o644))

	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	assert.Equal(t, "dev", Current(nested))
}

func TestCurrentWorktreePointer(t *testing.T) {
	t.Parallel()
	realGitDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(realGitDir, "HEAD"), []byte("ref: refs/heads/wt\n"), 0o644))

	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+realGitDir+"\n"), 0o644))

	assert.Equal(t, "wt", Current(work))
}

func TestCurrentNoRepo(t *testing.T) {
	t.Parallel()
	assert.Empty(t, Current(t.TempDir()))
}

func TestCurrentEmptyDir(t *testing.T) {
	t.Parallel()
	assert.Empty(t, Current(""))
}
