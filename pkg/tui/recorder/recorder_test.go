package recorder

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeModel is a minimal tea.Model whose render is controlled by the test, so
// frame snapshots can be asserted precisely.
type fakeModel struct {
	content string
	program *tea.Program
}

func (m *fakeModel) Init() tea.Cmd                       { return nil }
func (m *fakeModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m *fakeModel) View() tea.View                      { return tea.NewView(m.content) }
func (m *fakeModel) SetProgram(p *tea.Program)           { m.program = p }

func key(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func TestRecorder_CapturesInputWithPreEventFrame(t *testing.T) {
	inner := &fakeModel{content: "first frame"}
	r := New(inner)

	r.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	r.Update(key('a', "a"))

	inner.content = "second frame"
	r.Update(tea.MouseClickMsg{X: 3, Y: 0, Button: tea.MouseLeft})

	require.Len(t, r.events, 2)
	assert.Equal(t, 100, r.width)
	assert.Equal(t, 30, r.height)

	assert.Equal(t, keyEvent, r.events[0].kind)
	assert.Equal(t, "first frame", r.events[0].frame, "frame must be captured before the event is applied")

	assert.Equal(t, clickEvent, r.events[1].kind)
	assert.Equal(t, 3, r.events[1].x)
	assert.Equal(t, "second frame", r.events[1].frame)
}

func TestRecorder_IgnoresNonLeftClicksAndOtherMessages(t *testing.T) {
	r := New(&fakeModel{})

	r.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseRight})
	r.Update(tea.MouseMotionMsg{X: 1, Y: 1})
	r.Update("some runtime event")

	assert.False(t, r.HasInput())
}

func TestRecorder_ReturnsWrapperFromUpdate(t *testing.T) {
	r := New(&fakeModel{})

	model, _ := r.Update(key('a', "a"))

	assert.Same(t, r, model, "Update must keep the wrapper in the loop or recording stops after one message")
}

func TestRecorder_ForwardsSetProgram(t *testing.T) {
	inner := &fakeModel{}
	r := New(inner)

	p := &tea.Program{}
	r.SetProgram(p)

	assert.Same(t, p, inner.program)
}

func TestRecorder_StripsANSIFromFrames(t *testing.T) {
	inner := &fakeModel{content: "\x1b[31mred text\x1b[0m   "}
	r := New(inner)

	r.Update(key('a', "a"))

	require.Len(t, r.events, 1)
	assert.Equal(t, "red text", r.events[0].frame)
}
