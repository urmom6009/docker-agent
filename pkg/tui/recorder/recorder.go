// Package recorder turns a live `docker-agent run --record` TUI session into a
// ready-to-edit tuitest e2e test.
//
// It wraps the real top-level TUI model and, for every keystroke and mouse
// click, records the input together with a snapshot of the frame the user was
// looking at when they acted. After the session ends, GenerateTest replays
// those events into the pkg/tui/tuitest DSL: typing bursts become Type(...),
// special keys become Press(...), clicks become ClickText(...), and the frame
// snapshots are diffed to synthesize WaitFor(Contains(...)) synchronization
// points. Combined with the model-traffic cassette recorded by the --record
// proxy, the output is a complete offline regression test.
package recorder

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type eventKind int

const (
	keyEvent eventKind = iota
	clickEvent
)

// event is one recorded user input plus the frame the user was looking at
// when they acted (captured before the input is applied to the model).
type event struct {
	kind  eventKind
	key   tea.KeyPressMsg
	x, y  int
	at    time.Time
	frame string
}

// Recorder is a tea.Model wrapping the real TUI model. All tea.Model methods
// run on the Bubble Tea message-loop goroutine; GenerateTest must only be
// called after Program.Run has returned.
type Recorder struct {
	inner         tea.Model
	events        []event
	width, height int
	now           func() time.Time // test seam
}

// New wraps inner so its user input is recorded.
func New(inner tea.Model) *Recorder {
	return &Recorder{inner: inner, now: time.Now}
}

func (r *Recorder) Init() tea.Cmd { return r.inner.Init() }

func (r *Recorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		if m.Width > 0 && m.Height > 0 {
			r.width, r.height = m.Width, m.Height
		}
	case tea.KeyPressMsg:
		r.events = append(r.events, event{kind: keyEvent, key: m, at: r.now(), frame: r.frame()})
	case tea.MouseClickMsg:
		if m.Button == tea.MouseLeft {
			r.events = append(r.events, event{kind: clickEvent, x: m.X, y: m.Y, at: r.now(), frame: r.frame()})
		}
	}
	inner, cmd := r.inner.Update(msg)
	r.inner = inner
	return r, cmd
}

func (r *Recorder) View() tea.View { return r.inner.View() }

// SetProgram forwards the running program to the wrapped model so the session
// supervisor can route runtime events back through it.
func (r *Recorder) SetProgram(p *tea.Program) {
	if pr, ok := r.inner.(interface{ SetProgram(p *tea.Program) }); ok {
		pr.SetProgram(p)
	}
}

// HasInput reports whether any user input was recorded. When false there is
// nothing worth generating a test from.
func (r *Recorder) HasInput() bool { return len(r.events) > 0 }

// frame renders the wrapped model with ANSI codes stripped and trailing
// whitespace trimmed, matching tuitest's frame normalization so anchors
// derived from recorded frames match replayed frames.
func (r *Recorder) frame() string {
	lines := strings.Split(ansi.Strip(r.inner.View().Content), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}
