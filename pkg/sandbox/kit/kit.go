// Package kit stages a docker-agent kit on the host before launching a
// sandbox.
//
// A kit is a self-contained directory that bundles every host resource
// an agent will need at runtime — local skills, AGENTS.md/CLAUDE.md
// prompt files, sub-agent YAMLs — laid out under a fixed schema:
//
//	<kit>/skills/<skill-name>/         # local skills, recursively
//	<kit>/prompt_files/<name>          # collected add_prompt_files inputs
//	<kit>/manifest.json                # debug + cache key
//
// The host stages the kit, redacts secrets via [portcullis.Redact] in
// every text file, and bind-mounts the kit read-only inside the sandbox.
// At runtime, the in-sandbox resolvers ([skills.Load],
// [promptfiles.Paths]) consult [skills.KitDirEnv] to read from the kit
// instead of the user's $HOME — which doesn't exist inside the sandbox.
//
// The kit solves four constraints inherent to the docker sandbox CLI:
//   - the user's $HOME inside the sandbox is unrelated to the host's;
//   - sandbox mounts target directories, not individual files;
//   - host files may contain secrets that must not leak;
//   - other host-only state (e.g. .agents/skills under a parent dir) is
//     unreachable from the sandbox.
package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/docker/portcullis"

	"github.com/docker/docker-agent/pkg/config"
	latestcfg "github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/paths"
	"github.com/docker/docker-agent/pkg/promptfiles"
	"github.com/docker/docker-agent/pkg/skills"
)

// MountPath is the path at which the kit is bind-mounted inside the sandbox.
const MountPath = "/agent-kit"

// Options describes a kit build.
type Options struct {
	// AgentRef is the user-facing reference to the agent (a YAML path,
	// an OCI ref, a URL or a builtin name). Used for cache keying and
	// for loading the team config so the kit knows which prompt files
	// and skills to ship.
	AgentRef string

	// EnvProvider is forwarded to [config.Resolve] so URL-sourced
	// agents can pick up GITHUB_TOKEN. May be nil.
	EnvProvider environment.Provider

	// HostCwd is the host's working directory. Prompt-file lookups
	// walk up from it the same way the in-sandbox runtime would.
	HostCwd string

	// HostHome overrides the host's $HOME for prompt-file lookups.
	// Empty means "use os.UserHomeDir".
	HostHome string

	// Workspace is the absolute host path that the sandbox mounts as
	// the agent's live working directory. Files under it are not
	// staged into the kit because the sandbox sees them through the
	// live mount; staging them would duplicate content and ship a
	// stale, redacted copy alongside the live one.
	Workspace string

	// CacheDir is the parent directory under which the kit will be
	// staged. Empty means "use [paths.GetCacheDir]/sandbox-kits".
	CacheDir string
}

// Result is what [Build] returns.
type Result struct {
	// HostDir is the absolute host path of the staged kit. Mount it
	// read-only at [MountPath] inside the sandbox and forward
	// `-e DOCKER_AGENT_KIT_DIR=<MountPath>` so the in-sandbox
	// resolvers find it.
	HostDir string

	// Manifest describes what was staged.
	Manifest Manifest
}

// Manifest is the kit's table of contents. It is also written to
// <HostDir>/manifest.json for debugging.
type Manifest struct {
	AgentRef    string      `json:"agent_ref"`
	BuiltAt     time.Time   `json:"built_at"`
	Skills      []Entry     `json:"skills,omitempty"`
	PromptFiles []Entry     `json:"prompt_files,omitempty"`
	Redactions  []Redaction `json:"redactions,omitempty"`
}

// Entry records one staged file or directory.
type Entry struct {
	// Source is the original host path.
	Source string `json:"source"`
	// Target is the path relative to the kit root.
	Target string `json:"target"`
}

// Redaction records that portcullis added at least one [portcullis.Marker]
// to the staged copy of a file.
type Redaction struct {
	// Source is the host path of the original file.
	Source string `json:"source"`
	// Target is the path relative to the kit root.
	Target string `json:"target"`
}

// Build stages the kit and returns its location.
//
// The kit directory is reused across runs (deterministic path keyed by
// AgentRef) but always rebuilt fresh, so callers do not need to invalidate
// it manually when a skill or AGENTS.md changes on the host.
func Build(ctx context.Context, opts Options) (*Result, error) {
	hostHome := opts.HostHome
	if hostHome == "" {
		hostHome, _ = os.UserHomeDir()
	}

	cacheParent := opts.CacheDir
	if cacheParent == "" {
		cacheParent = filepath.Join(paths.GetCacheDir(), "sandbox-kits")
	}

	kitDir := filepath.Join(cacheParent, hashKey(opts.AgentRef))
	if err := resetDir(kitDir); err != nil {
		return nil, fmt.Errorf("preparing kit dir: %w", err)
	}

	manifest := Manifest{
		AgentRef: opts.AgentRef,
		BuiltAt:  time.Now().UTC(),
	}

	// Load the team config so we know which prompt files / skills the
	// agent will request. A failure here is non-fatal: we still want
	// to ship local skills since they're discovered from $HOME, not
	// from the config. We log and continue with an empty config.
	cfg, err := loadConfig(ctx, opts)
	if err != nil {
		slog.DebugContext(ctx, "kit: agent config unavailable; skipping prompt-file collection", "err", err)
		cfg = &latestcfg.Config{}
	}

	skillsEntries, redactions, err := stageSkills(kitDir)
	if err != nil {
		return nil, err
	}
	manifest.Skills = skillsEntries
	manifest.Redactions = append(manifest.Redactions, redactions...)

	promptEntries, redactions, err := stagePromptFiles(kitDir, cfg, opts.HostCwd, hostHome, opts.Workspace)
	if err != nil {
		return nil, err
	}
	manifest.PromptFiles = promptEntries
	manifest.Redactions = append(manifest.Redactions, redactions...)

	if err := writeManifest(kitDir, manifest); err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "kit: built",
		"dir", kitDir,
		"skills", len(manifest.Skills),
		"prompt_files", len(manifest.PromptFiles),
		"redactions", len(manifest.Redactions))

	return &Result{HostDir: kitDir, Manifest: manifest}, nil
}

// hashKey turns AgentRef into a short, filesystem-safe directory name.
// We hash rather than sanitise the ref so OCI refs (which contain ":"
// and "/") and absolute paths share the same encoding.
func hashKey(ref string) string {
	if ref == "" {
		ref = "default"
	}
	sum := sha256.Sum256([]byte(ref))
	return hex.EncodeToString(sum[:8])
}

// resetDir ensures dir exists and is empty. Used at the start of every
// Build so a previous run's stale entries cannot leak in.
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o750)
}

func loadConfig(ctx context.Context, opts Options) (*latestcfg.Config, error) {
	source, err := config.Resolve(opts.AgentRef, opts.EnvProvider)
	if err != nil {
		return nil, err
	}
	return config.Load(ctx, source)
}

// stageSkills copies every local skill discovered on the host into
// <kit>/skills/<skill-name>/, redacting text files in place.
func stageSkills(kitDir string) ([]Entry, []Redaction, error) {
	target := filepath.Join(kitDir, skills.KitSkillsSubdir)
	if err := os.MkdirAll(target, 0o750); err != nil {
		return nil, nil, fmt.Errorf("creating kit skills dir: %w", err)
	}

	var (
		entries    []Entry
		redactions []Redaction
	)
	for _, skill := range skills.Load([]string{"local"}) {
		if skill.BaseDir == "" {
			continue
		}
		dst := filepath.Join(target, sanitise(skill.Name))
		reds, err := copyTree(skill.BaseDir, dst)
		if err != nil {
			return nil, nil, fmt.Errorf("staging skill %q: %w", skill.Name, err)
		}
		entries = append(entries, Entry{
			Source: skill.BaseDir,
			Target: filepath.Join(skills.KitSkillsSubdir, sanitise(skill.Name)),
		})
		redactions = append(redactions, reds...)
	}
	return entries, redactions, nil
}

// stagePromptFiles walks every agent in cfg and copies each
// add_prompt_files entry that lives outside the live workspace into
// <kit>/prompt_files/<name>. Files under workspace are skipped because
// the sandbox already mounts that directory live; shipping a redacted
// duplicate would only confuse the in-sandbox cwd-walk lookup.
func stagePromptFiles(kitDir string, cfg *latestcfg.Config, hostCwd, hostHome, workspace string) ([]Entry, []Redaction, error) {
	target := filepath.Join(kitDir, promptfiles.KitSubdir)
	if err := os.MkdirAll(target, 0o750); err != nil {
		return nil, nil, fmt.Errorf("creating kit prompt-files dir: %w", err)
	}

	var (
		entries    []Entry
		redactions []Redaction
		seen       = make(map[string]bool)
	)
	for _, agent := range cfg.Agents {
		for _, name := range agent.AddPromptFiles {
			if seen[name] {
				continue
			}
			seen[name] = true

			for _, src := range promptfiles.Paths(hostCwd, hostHome, "", name) {
				if isUnder(src, workspace) {
					// The live mount will surface it inside the sandbox.
					continue
				}
				rel := filepath.Join(promptfiles.KitSubdir, name)
				dst := filepath.Join(kitDir, rel)
				red, err := copyFile(src, dst)
				if err != nil {
					return nil, nil, fmt.Errorf("staging prompt file %q: %w", src, err)
				}
				entries = append(entries, Entry{Source: src, Target: rel})
				if red != nil {
					redactions = append(redactions, *red)
				}
				// Only ship the closest match; a second one would
				// overwrite the kit copy without distinguishing them.
				break
			}
		}
	}
	return entries, redactions, nil
}

// isUnder reports whether path is contained within base. Both paths are
// resolved to absolute form before comparison so that "../foo" and
// symlinked traversals don't escape detection.
func isUnder(path, base string) bool {
	if base == "" {
		return false
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sanitise replaces filesystem-unfriendly characters in a skill name so
// it becomes a usable directory entry under <kit>/skills/. We only
// replace separators; the rest is allowed because skills loaded from
// disk already had filesystem-friendly names.
func sanitise(name string) string {
	r := strings.NewReplacer(string(filepath.Separator), "_", "..", "_", "/", "_", "\\", "_")
	return r.Replace(name)
}

// copyTree copies the directory rooted at src to dst recursively,
// applying [portcullis.Redact] to every text file. Symlinks inside the
// tree are followed for files (the contents are inlined into the kit)
// but not for directories — a symlink-to-directory is skipped to avoid
// staging arbitrary host content the user did not ask for.
func copyTree(src, dst string) ([]Redaction, error) {
	var redactions []Redaction
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		out := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(out, 0o750)
		case d.Type()&fs.ModeSymlink != 0:
			// Resolve the symlink. Skip if it points outside src to
			// avoid staging arbitrary host content.
			target, terr := filepath.EvalSymlinks(path)
			if terr != nil {
				return nil
			}
			info, sterr := os.Stat(target)
			if sterr != nil || info.IsDir() {
				return nil
			}
			red, copyErr := copyFile(target, out)
			if copyErr != nil {
				return copyErr
			}
			if red != nil {
				redactions = append(redactions, *red)
			}
			return nil
		default:
			red, copyErr := copyFile(path, out)
			if copyErr != nil {
				return copyErr
			}
			if red != nil {
				redactions = append(redactions, *red)
			}
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return redactions, nil
}

// copyFile copies a regular file from src to dst, redacting via
// [portcullis.Redact] when src is detected as text. Returns a non-nil
// [Redaction] when at least one secret was scrubbed.
func copyFile(src, dst string) (*Redaction, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return nil, err
	}

	out := data
	var redaction *Redaction
	if isText(data) {
		original := string(data)
		scrubbed := portcullis.Redact(original)
		if scrubbed != original {
			out = []byte(scrubbed)
			redaction = &Redaction{Source: src, Target: dst}
		}
	}

	if err := os.WriteFile(dst, out, 0o600); err != nil {
		return nil, err
	}
	return redaction, nil
}

// isText reports whether b looks like a text file. The heuristic is
// deliberately strict: NUL byte → binary, invalid UTF-8 → binary. This
// errs on the side of not feeding [portcullis.Redact] non-text input
// (which would either corrupt binaries or produce nonsense output).
func isText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	// Strip a UTF-8 BOM if present so it doesn't trip up the UTF-8 check.
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	if slices.Contains(b, 0) {
		return false
	}
	return utf8.Valid(b)
}

// writeManifest serialises the manifest as pretty-printed JSON. The
// kit directory is meant to be inspected by humans during debugging,
// so readability beats compactness.
func writeManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}
