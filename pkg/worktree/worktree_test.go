package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/paths"
)

func TestCreateWorktree(t *testing.T) {
	dir := bootstrapRepo(t)
	dataDir := t.TempDir()
	paths.SetDataDir(dataDir)
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "")
	require.NoError(t, err)
	require.NotNil(t, wt)

	assert.DirExists(t, wt.Dir)
	// A random name looks like "focused_turing" (adjective_surname).
	assert.Regexp(t, `^[a-z]+_[a-z]+$`, wt.Name)
	assert.Equal(t, "worktree-"+wt.Name, wt.Branch)
	assert.Equal(t, filepath.Join(dataDir, "worktrees", wt.Name), wt.Dir)

	// The worktree shares the repository's history: the initial commit's
	// files must be present in the new working directory.
	assert.FileExists(t, filepath.Join(wt.Dir, "a.txt"))

	// The checked-out branch must match the one reported by the worktree.
	out := gitOut(t, wt.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	assert.Equal(t, wt.Branch, out)
}

func TestCreateWithName(t *testing.T) {
	dir := bootstrapRepo(t)
	dataDir := t.TempDir()
	paths.SetDataDir(dataDir)
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "my-feature")
	require.NoError(t, err)

	assert.Equal(t, "my-feature", wt.Name)
	assert.Equal(t, "worktree-my-feature", wt.Branch)
	assert.Equal(t, filepath.Join(dataDir, "worktrees", "my-feature"), wt.Dir)
	assert.FileExists(t, filepath.Join(wt.Dir, "a.txt"))
}

func TestCreateFromSubfolder(t *testing.T) {
	root := bootstrapRepo(t)
	sub := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), sub, "")
	require.NoError(t, err)
	assert.DirExists(t, wt.Dir)
	assert.FileExists(t, filepath.Join(wt.Dir, "a.txt"))
}

func TestCreateOutsideGitRepo(t *testing.T) {
	_, err := Create(t.Context(), t.TempDir(), "")
	assert.ErrorIs(t, err, ErrNotGitRepository)
}

func TestCreateRejectsUnsafeNames(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	for _, name := range []string{
		"../escape",
		"../../etc/evil",
		"foo/bar",
		`foo\bar`,
		".",
		"..",
		" leading",
		"trailing ",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Create(t.Context(), dir, name)
			assert.ErrorIs(t, err, ErrInvalidName)
		})
	}
}

func TestCreateRejectsDuplicateName(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	_, err := Create(t.Context(), dir, "dup")
	require.NoError(t, err)

	_, err = Create(t.Context(), dir, "dup")
	assert.ErrorIs(t, err, ErrInvalidName)
}

func TestStatusClean(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "clean")
	require.NoError(t, err)

	st, err := wt.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.IsDirty())
	assert.False(t, st.Modified)
	assert.False(t, st.Untracked)
	assert.False(t, st.NewCommits)
}

func TestStatusDetectsUntracked(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "untracked")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt.Dir, "new.txt"), []byte("x"), 0o644))

	st, err := wt.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.IsDirty())
	assert.True(t, st.Untracked)
	assert.False(t, st.Modified)
	assert.False(t, st.NewCommits)
}

func TestStatusDetectsModified(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "modified")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt.Dir, "a.txt"), []byte("changed"), 0o644))

	st, err := wt.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.IsDirty())
	assert.True(t, st.Modified)
	assert.False(t, st.Untracked)
	assert.False(t, st.NewCommits)
}

func TestStatusDetectsNewCommits(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "committed")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt.Dir, "a.txt"), []byte("changed"), 0o644))
	runGit(t, wt.Dir, "commit", "-am", "work")

	st, err := wt.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.IsDirty())
	assert.True(t, st.NewCommits)
	// A committed change leaves a clean tree.
	assert.False(t, st.Modified)
	assert.False(t, st.Untracked)
}

func TestRemove(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "gone")
	require.NoError(t, err)
	require.DirExists(t, wt.Dir)

	require.NoError(t, wt.Remove(t.Context()))
	assert.NoDirExists(t, wt.Dir)

	// The branch must be gone too.
	branches := gitOut(t, dir, "branch", "--list", wt.Branch)
	assert.Empty(t, branches)
}

func TestRemoveDiscardsDirtyWork(t *testing.T) {
	dir := bootstrapRepo(t)
	paths.SetDataDir(t.TempDir())
	t.Cleanup(func() { paths.SetDataDir("") })

	wt, err := Create(t.Context(), dir, "dirty")
	require.NoError(t, err)
	// Uncommitted change + untracked file + a new commit: removal must
	// still succeed and wipe everything.
	require.NoError(t, os.WriteFile(filepath.Join(wt.Dir, "a.txt"), []byte("changed"), 0o644))
	runGit(t, wt.Dir, "commit", "-am", "work")
	require.NoError(t, os.WriteFile(filepath.Join(wt.Dir, "untracked.txt"), []byte("x"), 0o644))

	require.NoError(t, wt.Remove(t.Context()))
	assert.NoDirExists(t, wt.Dir)
}

func bootstrapRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	// Keep the test hermetic: never sign commits with the developer's
	// global signing setup (e.g. a 1Password SSH agent), which is
	// unavailable/flaky in test environments and breaks `git commit`.
	runGit(t, dir, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(trimNL(out))
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func TestParsePRRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{in: "123", want: 123},
		{in: "#123", want: 123},
		{in: "  42 ", want: 42},
		{in: "https://github.com/owner/repo/pull/123", want: 123},
		{in: "https://github.com/owner/repo/pull/123/files", want: 123},
		{in: "https://github.com/owner/repo/pull/123#issuecomment-1", want: 123},
		{in: "https://github.com/owner/repo/pull/7?w=1", want: 7},
		{in: "", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "0", wantErr: true},
		{in: "-3", wantErr: true},
		{in: "https://github.com/owner/repo/issues/123", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := parsePRRef(tt.in)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidPRRef)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCreatePRValidatesRefBeforeGit checks a malformed ref fails fast with
// ErrInvalidPRRef, before touching git or gh, even outside a repository.
func TestCreatePRValidatesRefBeforeGit(t *testing.T) {
	_, err := CreatePR(t.Context(), t.TempDir(), "not-a-pr")
	assert.ErrorIs(t, err, ErrInvalidPRRef)
}
