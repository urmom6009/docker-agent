package root

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/docker/docker-agent/pkg/cli"
	"github.com/docker/docker-agent/pkg/tui/recorder"
)

// writeGeneratedTUITest writes the tuitest e2e test generated from a recorded
// TUI session next to the cassette. Failures are logged, not fatal: the
// cassette is the primary artifact and has already been saved.
func writeGeneratedTUITest(ctx context.Context, out *cli.Printer, rec *recorder.Recorder, cassettePath, agentFile string) {
	src := rec.GenerateTest(recorder.GenerateOptions{
		AgentFile:    agentFile,
		CassettePath: cassettePath,
	})

	path, err := writeUniqueFile(strings.TrimSuffix(cassettePath, ".yaml"), []byte(src))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to write generated TUI test", "error", err)
		return
	}
	out.Println("Generated TUI test: " + path + " (see its header comment to make it runnable)")
}

// writeUniqueFile writes content to <base>_test.go, or <base>_N_test.go when
// that already exists, and never overwrites: --record paths are
// user-controlled, so silently clobbering an unrelated *_test.go would be
// destructive.
func writeUniqueFile(base string, content []byte) (string, error) {
	for n := 1; n <= 100; n++ {
		path := base + "_test.go"
		if n > 1 {
			path = fmt.Sprintf("%s_%d_test.go", base, n)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		// Remove on failure: a leftover empty file would permanently occupy
		// this O_EXCL slot on every subsequent --record run.
		if _, err := f.Write(content); err != nil {
			f.Close()
			os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("no available file name for %s_test.go", base)
}
