package backgroundagent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/session"
	"github.com/docker/docker-agent/pkg/tools"
	agenttool "github.com/docker/docker-agent/pkg/tools/builtin/agent"
	"github.com/docker/docker-agent/pkg/tui/core/layout"
	"github.com/docker/docker-agent/pkg/tui/service"
	"github.com/docker/docker-agent/pkg/tui/types"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// brailleSpinnerRx matches any braille-range code point used by the shared
// spinner so tests are not tied to a specific animation frame.
var brailleSpinnerRx = regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]`)

func sessionState() service.SessionStateReader {
	return service.NewSessionState(&session.Session{})
}

func toolCallMessage(name, args, result string, status types.ToolStatus) *types.Message {
	return &types.Message{
		Type:           types.MessageTypeToolCall,
		Sender:         "root",
		ToolStatus:     status,
		Content:        result,
		ToolDefinition: tools.Tool{Name: name},
		ToolCall: tools.ToolCall{
			Function: tools.FunctionCall{Name: name, Arguments: args},
		},
	}
}

func render(view layout.Model) string {
	view.SetSize(80, 1)
	return stripANSI(view.View())
}

// TestRunCard_StatusIcon locks in the dispatch card: a running or pending
// run_background_agent animates a braille spinner (never a premature ✓), while
// completed/error states show ✓/✗. The "<sender> dispatches <agent>" header and
// the task text must render in every state.
func TestRunCard_StatusIcon(t *testing.T) {
	t.Parallel()

	const args = `{"agent":"researcher","task":"Investigate the flaky test"}`

	tests := []struct {
		name      string
		status    types.ToolStatus
		wantGlyph string // exact glyph; empty means any braille spinner frame
		notWant   []string
	}{
		{"running animates spinner", types.ToolStatusRunning, "", []string{"✓", "✗"}},
		{"pending animates spinner", types.ToolStatusPending, "", []string{"✓", "✗"}},
		{"completed shows check", types.ToolStatusCompleted, "✓", []string{"✗"}},
		{"error shows cross", types.ToolStatusError, "✗", []string{"✓"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := toolCallMessage(agenttool.ToolNameRunBackgroundAgent, args, "", tc.status)
			out := render(NewRun(msg, sessionState()))

			assert.Regexp(t, `root\s+dispatches\s+researcher`, out, "header should name dispatcher and agent")
			assert.Contains(t, out, "Investigate the flaky test", "task text should always render")

			if tc.wantGlyph == "" {
				// In-flight states must show a braille spinner, not a terminal glyph.
				assert.Regexp(t, brailleSpinnerRx, out, "expected braille spinner glyph for in-flight status")
				assert.NotRegexp(t, brailleSpinnerRx, strings.Join(tc.notWant, ""), "test case invariant")
			} else {
				assert.Contains(t, out, tc.wantGlyph, "expected status glyph missing")
				assert.NotRegexp(t, brailleSpinnerRx, out, "spinner glyph should not appear for terminal status")
			}
			for _, n := range tc.notWant {
				assert.NotContains(t, out, n, "unexpected glyph %q present for status %s", n, tc.name)
			}
		})
	}
}

// TestRunCard_IconWidthIsStable guards the fixed-width status-icon contract
// shared with the transfertask card: the icon is a single glyph with no
// elapsed-time suffix, so the wrapped task text keeps the same indent across
// statuses.
func TestRunCard_IconWidthIsStable(t *testing.T) {
	t.Parallel()

	const args = `{"agent":"researcher","task":"Investigate the flaky test"}`

	taskColumn := func(status types.ToolStatus) int {
		msg := toolCallMessage(agenttool.ToolNameRunBackgroundAgent, args, "", status)
		// Layout is "header\n\n<icon> task", so the task block is the 3rd segment.
		parts := strings.SplitN(render(NewRun(msg, sessionState())), "\n", 3)
		require.Len(t, parts, 3, "render output must have header, blank line, and task block (status=%s)", status)
		return strings.Index(parts[2], "Investigate")
	}

	running := taskColumn(types.ToolStatusRunning)
	assert.Positive(t, running, "task text should be indented past the icon")
	assert.Equal(t, running, taskColumn(types.ToolStatusPending))
	assert.Equal(t, running, taskColumn(types.ToolStatusCompleted))
	assert.Equal(t, running, taskColumn(types.ToolStatusError))
}

// TestViewAndStop_ShowTaskIDAndResult verifies the view/stop renderers surface
// the task id (argument) plus the already-formatted result text once complete.
func TestViewAndStop_ShowTaskIDAndResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		toolName   string
		taskID     string
		result     string
		resultText string // a distinctive token that must survive rendering
		newView    func(*types.Message, service.SessionStateReader) layout.Model
	}{
		{
			name:       "view shows id and output",
			toolName:   agenttool.ToolNameViewBackgroundAgent,
			taskID:     "agent_task_42",
			result:     "Status:  completed\n--- Output ---\nfound the flaky test",
			resultText: "found the flaky test",
			newView:    NewView,
		},
		{
			name:       "stop shows id and confirmation",
			toolName:   agenttool.ToolNameStopBackgroundAgent,
			taskID:     "agent_task_7",
			result:     "Background agent task\nagent_task_7 stopped.",
			resultText: "stopped.",
			newView:    NewStop,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := `{"task_id":"` + tc.taskID + `"}`
			msg := toolCallMessage(tc.toolName, args, tc.result, types.ToolStatusCompleted)
			out := render(tc.newView(msg, sessionState()))

			assert.Contains(t, out, tc.taskID, "task id should render as the argument")
			assert.Contains(t, out, tc.resultText, "result text should render")
		})
	}
}

// TestList_ShowsResult verifies list_background_agents surfaces the tool result
// (the task listing) once complete, and stays quiet while still running.
func TestList_ShowsResult(t *testing.T) {
	t.Parallel()

	const result = "Background Agent Tasks:\n\nID: agent_task_1\n  Agent:   researcher\n  Status:  running"

	completed := toolCallMessage(agenttool.ToolNameListBackgroundAgents, "", result, types.ToolStatusCompleted)
	out := render(NewList(completed, sessionState()))
	assert.Contains(t, out, "agent_task_1", "list should render the task listing result")
	assert.Contains(t, out, "researcher", "list should render task details from the result")

	running := toolCallMessage(agenttool.ToolNameListBackgroundAgents, "", result, types.ToolStatusRunning)
	assert.NotContains(t, render(NewList(running, sessionState())), "agent_task_1",
		"result should only show once the call completes")
}

// TestRunCard_PartialJSONArgs verifies that renderRun produces a non-empty card
// when the tool-call arguments JSON is still streaming (partial). This exercises
// the toolcommon.ParseArgs path which closes unclosed JSON before unmarshalling.
func TestRunCard_PartialJSONArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    string
		wantHdr string // substring that must appear in the header
	}{
		{
			name:    "agent field complete, task still streaming",
			args:    `{"agent":"researcher","task":"Investig`,
			wantHdr: "researcher",
		},
		{
			name:    "agent field complete, no task yet",
			args:    `{"agent":"researcher"`,
			wantHdr: "researcher",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := toolCallMessage(agenttool.ToolNameRunBackgroundAgent, tc.args, "", types.ToolStatusPending)
			out := render(NewRun(msg, sessionState()))
			assert.NotEmpty(t, out, "card must render even with partial-JSON args")
			assert.Contains(t, out, tc.wantHdr, "agent name must appear in card header")
		})
	}
}
