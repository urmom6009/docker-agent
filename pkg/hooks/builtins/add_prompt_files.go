package builtins

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/docker/docker-agent/pkg/hooks"
	"github.com/docker/docker-agent/pkg/promptfiles"
)

// AddPromptFiles is the registered name of the add_prompt_files builtin.
const AddPromptFiles = "add_prompt_files"

// addPromptFiles reads each filename in args from the workdir hierarchy
// and the user's home directory (or the staged kit when running in a
// sandbox), joining their contents into a turn_start AdditionalContext.
// Missing or unreadable files are logged and skipped; surviving files
// still contribute.
func addPromptFiles(_ context.Context, in *hooks.Input, args []string) (*hooks.Output, error) {
	if in == nil || in.Cwd == "" || len(args) == 0 {
		return nil, nil
	}
	home, _ := os.UserHomeDir() // empty string disables the home-dir lookup
	var parts []string
	for _, name := range args {
		for _, path := range promptfiles.PathsFromEnv(in.Cwd, home, name) {
			content, err := os.ReadFile(path)
			if err != nil {
				slog.Warn("reading prompt file", "path", path, "error", err)
				continue
			}
			parts = append(parts, string(content))
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return hooks.NewAdditionalContextOutput(hooks.EventTurnStart, strings.Join(parts, "\n\n")), nil
}
