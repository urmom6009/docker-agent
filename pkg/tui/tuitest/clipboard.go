package tuitest

import (
	"strings"
	"sync"
	"time"

	"github.com/docker/docker-agent/pkg/tui/components/messages"
)

type clipboardStore struct {
	mu     sync.Mutex
	values []string
}

func newClipboardStore() (*clipboardStore, func()) {
	store := &clipboardStore{}
	restore := messages.SetClipboardWriterForTest(func(s string) error {
		store.mu.Lock()
		store.values = append(store.values, s)
		store.mu.Unlock()
		return nil
	})
	return store, restore
}

func (s *clipboardStore) latest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		return ""
	}
	return s.values[len(s.values)-1]
}

// Clipboard returns the latest text copied through the TUI's clipboard path.
// The tuitest driver installs a spy so tests never touch the real system
// clipboard.
func (d *Driver) Clipboard() string {
	return d.clipboard.latest()
}

// WaitForClipboard polls until the latest copied text contains substr, or the
// driver's timeout elapses.
func (d *Driver) WaitForClipboard(substr string) *Driver {
	d.tb.Helper()

	deadline := time.Now().Add(d.waitTimeout)
	for time.Now().Before(deadline) {
		if strings.Contains(d.clipboard.latest(), substr) {
			return d
		}
		time.Sleep(pollInterval)
	}

	d.tb.Fatalf("tuitest: timed out after %s waiting for clipboard to contain %q\n--- clipboard ---\n%s",
		d.waitTimeout, substr, d.clipboard.latest())
	return d
}

// AssertClipboard immediately checks the latest copied text with m.
func (d *Driver) AssertClipboard(m Matcher) *Driver {
	d.tb.Helper()
	clip := d.clipboard.latest()
	if !m.match(clip) {
		d.tb.Fatalf("tuitest: clipboard assertion failed: %s\n--- clipboard ---\n%s", m.describe(), clip)
	}
	return d
}
