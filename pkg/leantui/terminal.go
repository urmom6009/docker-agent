package leantui

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

// terminal owns the raw-mode TTY: it switches the input file descriptor into
// raw mode, exposes a cancelable reader for keyboard input, a buffered writer
// for output, and the current window size. It deliberately never enters the
// alternate screen so the conversation is written to (and scrolls in) the
// normal terminal buffer.
type terminal struct {
	in     *os.File
	out    *os.File
	writer *bufio.Writer
	reader cancelreader.CancelReader

	prevState *term.State

	width  int
	height int

	winch chan os.Signal
}

func newTerminal(in, out *os.File) (*terminal, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, fmt.Errorf("enabling raw mode: %w", err)
	}

	reader, err := cancelreader.NewReader(in)
	if err != nil {
		_ = term.Restore(int(in.Fd()), state)
		return nil, fmt.Errorf("creating input reader: %w", err)
	}

	t := &terminal{
		in:        in,
		out:       out,
		writer:    bufio.NewWriterSize(out, 16*1024),
		reader:    reader,
		prevState: state,
		winch:     make(chan os.Signal, 1),
	}

	t.width, t.height = t.querySize()
	if t.width <= 0 {
		t.width = 80
	}
	if t.height <= 0 {
		t.height = 24
	}

	t.writeString(seqEnableBracketedPaste)
	t.flush()

	signal.Notify(t.winch, syscall.SIGWINCH)

	return t, nil
}

// resized blocks until the terminal is resized, then refreshes the cached
// dimensions and reports the new size. It returns ok=false once the resize
// channel is closed during shutdown.
func (t *terminal) resized() (w, h int, ok bool) {
	if _, open := <-t.winch; !open {
		return 0, 0, false
	}
	t.width, t.height = t.querySize()
	if t.width <= 0 {
		t.width = 80
	}
	if t.height <= 0 {
		t.height = 24
	}
	return t.width, t.height, true
}

func (t *terminal) querySize() (w, h int) {
	w, h, err := term.GetSize(int(t.out.Fd()))
	if err != nil {
		return 0, 0
	}
	return w, h
}

func (t *terminal) size() (w, h int) {
	return t.width, t.height
}

func (t *terminal) writeString(s string) {
	_, _ = t.writer.WriteString(s)
}

func (t *terminal) flush() {
	_ = t.writer.Flush()
}

// restore tears the terminal back down: it disables bracketed paste, cancels
// the reader, restores the saved terminal state and stops listening for
// resize signals.
func (t *terminal) restore() {
	t.writeString(seqDisableBracketedPaste)
	t.writeString(seqShowCursor)
	t.flush()

	signal.Stop(t.winch)
	close(t.winch)

	t.reader.Cancel()
	_ = t.reader.Close()

	_ = term.Restore(int(t.in.Fd()), t.prevState)
}
