//go:build unix

package plan

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanTool_UpdateFromFileRejectsNamedPipe proves a non-regular file (here a
// named pipe with no writer) is rejected at the stat stage and never opened.
// Opening such a pipe would block forever and reading a device like /dev/zero
// would stream unbounded data, so the implementation must refuse it up front.
// The timeout guards against a regression that opens the file and hangs.
func TestPlanTool_UpdateFromFileRejectsNamedPipe(t *testing.T) {
	t.Parallel()
	tool := newTestPlanTool(t)

	fifo := filepath.Join(t.TempDir(), "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	ctx := t.Context()
	done := make(chan *struct {
		out     string
		isError bool
	}, 1)
	go func() {
		res, _ := tool.updatePlanFromFile(ctx, UpdatePlanFromFileArgs{Name: "p", Path: fifo})
		done <- &struct {
			out     string
			isError bool
		}{res.Output, res.IsError}
	}()

	select {
	case res := <-done:
		assert.True(t, res.isError)
		assert.Contains(t, res.out, "regular file")
	case <-time.After(5 * time.Second):
		t.Fatal("updatePlanFromFile blocked on a named pipe; it must reject non-regular files without opening them")
	}
}
