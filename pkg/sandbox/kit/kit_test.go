package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/portcullis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/promptfiles"
	"github.com/docker/docker-agent/pkg/skills"
)

// fakeGitHubToken is a syntactically valid GitHub PAT that triggers
// portcullis. It is only used as input to the redactor, never as an
// actual credential.
const fakeGitHubToken = "ghp_" + "1234567890abcdefghijklmnopqrstuvwxyz"

func TestBuild_StagesSkillsAndRedacts(t *testing.T) {
	hostHome := t.TempDir()

	// Stage one local skill on the host with a secret embedded.
	skillDir := filepath.Join(hostHome, ".agents", "skills", "secret-keeper")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	skillBody := "---\nname: secret-keeper\ndescription: ships with a secret\n---\n\ntoken=" + fakeGitHubToken + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644))

	t.Setenv("HOME", hostHome)
	// Run from an empty cwd so cwd-walking finds nothing extra.
	t.Chdir(t.TempDir())

	cacheDir := t.TempDir()
	res, err := Build(t.Context(), Options{
		AgentRef: "default",
		HostHome: hostHome,
		HostCwd:  t.TempDir(),
		CacheDir: cacheDir,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	// The kit is rooted under cacheDir.
	rel, err := filepath.Rel(cacheDir, res.HostDir)
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(rel, ".."),
		"kit dir %s should be under cache dir %s", res.HostDir, cacheDir)

	// The skill made it into the kit.
	staged := filepath.Join(res.HostDir, skills.KitSkillsSubdir, "secret-keeper", "SKILL.md")
	data, err := os.ReadFile(staged)
	require.NoError(t, err)
	assert.NotContains(t, string(data), fakeGitHubToken, "host secret must not survive in kit")
	assert.Contains(t, string(data), portcullis.Marker, "redaction marker must be present")

	// The manifest records the skill and the redaction.
	require.Len(t, res.Manifest.Skills, 1)
	assert.Equal(t, skillDir, res.Manifest.Skills[0].Source)
	assert.Equal(t, filepath.Join(skills.KitSkillsSubdir, "secret-keeper"), res.Manifest.Skills[0].Target)
	require.Len(t, res.Manifest.Redactions, 1)
	assert.Equal(t, filepath.Join(skillDir, "SKILL.md"), res.Manifest.Redactions[0].Source)

	// Manifest is also written to disk for human inspection.
	_, err = os.Stat(filepath.Join(res.HostDir, "manifest.json"))
	assert.NoError(t, err)
}

func TestBuild_RebuildsCleanDir(t *testing.T) {
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	t.Chdir(t.TempDir())

	cacheDir := t.TempDir()
	res1, err := Build(t.Context(), Options{
		AgentRef: "stable-ref",
		HostHome: hostHome,
		HostCwd:  t.TempDir(),
		CacheDir: cacheDir,
	})
	require.NoError(t, err)

	// Drop a stale file inside the kit dir; it must be wiped on rebuild.
	stale := filepath.Join(res1.HostDir, "stale.txt")
	require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o600))

	res2, err := Build(t.Context(), Options{
		AgentRef: "stable-ref",
		HostHome: hostHome,
		HostCwd:  t.TempDir(),
		CacheDir: cacheDir,
	})
	require.NoError(t, err)
	assert.Equal(t, res1.HostDir, res2.HostDir, "stable AgentRef should yield stable kit dir")

	_, err = os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "stale file must be removed when kit is rebuilt")
}

func TestBuild_PromptFilesCollectedAndScopedOutsideWorkspace(t *testing.T) {
	hostHome := t.TempDir()
	workspace := t.TempDir()

	// AGENTS.md at $HOME — must be staged into the kit.
	homeAgents := filepath.Join(hostHome, "AGENTS.md")
	require.NoError(t, os.WriteFile(homeAgents, []byte("# host AGENTS.md\n"), 0o600))

	// AGENTS.md inside the workspace — must NOT be staged because the
	// live mount surfaces it inside the sandbox.
	workspaceAgents := filepath.Join(workspace, "AGENTS.md")
	require.NoError(t, os.WriteFile(workspaceAgents, []byte("# workspace AGENTS.md\n"), 0o600))

	// Build a tiny agent YAML that references AGENTS.md via add_prompt_files.
	agentYAML := []byte(`#!/usr/bin/env docker-agent
agents:
  root:
    model: openai/gpt-5
    description: tester
    instruction: hello
    add_prompt_files: ["AGENTS.md"]
models:
  openai/gpt-5:
    provider: openai
    model: gpt-5
`)
	yamlPath := filepath.Join(workspace, "agent.yaml")
	require.NoError(t, os.WriteFile(yamlPath, agentYAML, 0o600))

	t.Setenv("HOME", hostHome)
	t.Chdir(workspace)

	cacheDir := t.TempDir()
	res, err := Build(t.Context(), Options{
		AgentRef:  yamlPath,
		HostHome:  hostHome,
		HostCwd:   workspace,
		Workspace: workspace,
		CacheDir:  cacheDir,
	})
	require.NoError(t, err)

	// Only the $HOME AGENTS.md is staged (the workspace one is reachable
	// live inside the sandbox).
	require.Len(t, res.Manifest.PromptFiles, 1)
	assert.Equal(t, homeAgents, res.Manifest.PromptFiles[0].Source)

	staged := filepath.Join(res.HostDir, promptfiles.KitSubdir, "AGENTS.md")
	data, err := os.ReadFile(staged)
	require.NoError(t, err)
	assert.Equal(t, "# host AGENTS.md\n", string(data))
}

func TestBuild_NoAgentRefLeavesPromptFilesEmpty(t *testing.T) {
	// Without an AgentRef there is no team config to walk; the kit
	// still builds (so the host-only skills lookup runs) but no
	// prompt files are staged.
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	t.Chdir(t.TempDir())

	cacheDir := t.TempDir()
	res, err := Build(t.Context(), Options{
		AgentRef: "", // unresolved on purpose
		HostHome: hostHome,
		HostCwd:  t.TempDir(),
		CacheDir: cacheDir,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Manifest.PromptFiles)
}

func TestIsUnder(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	inside := filepath.Join(base, "sub", "file")
	outside := t.TempDir()

	assert.True(t, isUnder(inside, base))
	assert.False(t, isUnder(outside, base))
	assert.False(t, isUnder(inside, ""), "empty base means no scoping")
}

func TestIsText(t *testing.T) {
	t.Parallel()

	assert.True(t, isText([]byte("hello world")))
	assert.True(t, isText([]byte{}))
	assert.True(t, isText([]byte("\xef\xbb\xbfhello"))) // UTF-8 BOM
	assert.False(t, isText([]byte{0x00, 0x01, 0x02}))   // NUL byte
	assert.False(t, isText([]byte{0xff, 0xfe, 0xfd}))   // invalid UTF-8
}

func TestSanitise(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "abc", sanitise("abc"))
	assert.Equal(t, "a_b", sanitise("a/b"))
	assert.Equal(t, "a_b", sanitise("a..b"))
}
