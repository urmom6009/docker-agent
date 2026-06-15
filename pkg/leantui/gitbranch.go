package leantui

import (
	"os"
	"path/filepath"
	"strings"
)

// gitBranch returns the current branch name (or short commit for a detached
// HEAD) for the repository containing dir, walking up parent directories until
// a .git entry is found. It returns "" when dir is not inside a repository.
func gitBranch(dir string) string {
	d := dir
	for {
		gitPath := filepath.Join(d, ".git")
		info, err := os.Stat(gitPath)
		switch {
		case err == nil && info.IsDir():
			return readHead(filepath.Join(gitPath, "HEAD"))
		case err == nil:
			// A .git file points at the real git dir (submodule or worktree).
			if gd := parseGitdir(gitPath, d); gd != "" {
				return readHead(filepath.Join(gd, "HEAD"))
			}
		}

		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func parseGitdir(gitFile, base string) string {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return ""
	}
	gd, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return ""
	}
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(base, gd)
	}
	return gd
}

func readHead(headPath string) string {
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(line, "ref: "); ok {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if len(line) >= 7 {
		return line[:7] // detached HEAD
	}
	return ""
}

// shortenPath replaces the user's home directory prefix with "~".
func shortenPath(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return dir
}
