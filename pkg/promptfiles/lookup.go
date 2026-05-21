// Package promptfiles centralises the search rules used to resolve
// add_prompt_files entries (typically AGENTS.md, CLAUDE.md, ...). The
// rules are shared by:
//
//   - the runtime add_prompt_files hook, which executes inside the
//     sandbox at turn-start;
//   - the docker-agent kit builder, which walks the same paths on the
//     host before sandbox launch so it can stage every relevant file
//     into the kit (see pkg/sandbox/kit).
//
// Keeping the rules in one place guarantees both ends pick the same
// files and avoids drift when, for example, the kit ships a redacted
// copy of ~/AGENTS.md that the in-sandbox lookup must then prefer over
// the now-missing host $HOME entry.
package promptfiles

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/docker/docker-agent/pkg/skills"
)

// KitSubdir is the subdirectory inside a docker-agent kit that holds
// staged prompt files. The host writes to it; the in-sandbox lookup
// reads from it.
const KitSubdir = "prompt_files"

// Paths returns the prompt-file paths to load for filename, in order:
//
//  1. The closest match found while walking up from workDir, if any.
//  2. Either a kitDir match (when kitDir is non-empty — running inside
//     a sandbox with a staged kit), or a homeDir match.
//
// kitDir takes precedence over homeDir because, in the sandbox, $HOME
// does not contain the host's prompt files; the kit is what brought
// them along (already redacted). Returns at most two paths. Passing
// homeDir == "" disables the home-dir lookup — useful in tests so
// they don't need to touch the real $HOME.
func Paths(workDir, homeDir, kitDir, filename string) []string {
	var paths []string
	if p := FindInHierarchy(workDir, filename); p != "" {
		paths = append(paths, p)
	}
	switch {
	case kitDir != "":
		p := filepath.Join(kitDir, KitSubdir, filename)
		if isFile(p) && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	case homeDir != "":
		p := filepath.Join(homeDir, filename)
		if isFile(p) && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	return paths
}

// PathsFromEnv is a convenience wrapper around Paths that reads the
// kit directory from [skills.KitDirEnv]. The runtime in-sandbox hook
// uses it; host-side callers (the kit builder) pass kitDir explicitly
// to Paths instead.
func PathsFromEnv(workDir, homeDir, filename string) []string {
	return Paths(workDir, homeDir, os.Getenv(skills.KitDirEnv), filename)
}

// FindInHierarchy searches for filename starting at startDir and
// walking up the directory tree. Returns the path of the first match,
// or "" if none.
func FindInHierarchy(startDir, filename string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		path := filepath.Join(dir, filename)
		if isFile(path) {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isFile reports whether path exists and is a regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
