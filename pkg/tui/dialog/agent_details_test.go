package dialog

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/config/types"
	"github.com/docker/docker-agent/pkg/runtime"
	"github.com/docker/docker-agent/pkg/tui/components/toolcommon"
	"github.com/docker/docker-agent/pkg/tui/styles"
)

// renderAgentDetails returns the ANSI-stripped rendered lines of the dialog
// body. cfg populates the inline config sections (toolsets, sub-agents,
// handoffs, fallbacks).
func renderAgentDetails(a runtime.AgentDetails, cfg runtime.AgentConfigInfo) string {
	d := NewAgentDetailsDialog(a, cfg).(*agentDetailsDialog)
	return ansi.Strip(strings.Join(d.renderLines(80, 24), "\n"))
}

func TestAgentDetailsDialog_RendersCoreFields(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{
		Name:        "root",
		Description: "Executive assistant that routes work",
		Provider:    "anthropic",
		Model:       "claude-opus-4-8",
		Thinking:    "high",
	}, runtime.AgentConfigInfo{})

	assert.Contains(t, out, "root")
	assert.Contains(t, out, "Executive assistant that routes work")
	assert.Contains(t, out, "Model: anthropic/claude-opus-4-8")
	// The thinking line shows the same six-cell gauge + value as the sidebar.
	assert.Contains(t, out, "Thinking: "+ansi.Strip(toolcommon.ThinkingGaugeValue("high")))
}

// TestAgentDetailsDialog_ThinkingVocabulary covers every thinking label kind,
// including the empty case (no thinking line). Each non-empty line carries the
// shared gauge + value rendering.
func TestAgentDetailsDialog_ThinkingVocabulary(t *testing.T) {
	t.Parallel()

	for _, thinking := range []string{"high", "adaptive", "8192", "off", ""} {
		t.Run(thinking, func(t *testing.T) {
			t.Parallel()
			out := renderAgentDetails(runtime.AgentDetails{
				Name:     "agent",
				Provider: "openai",
				Model:    "gpt-5.4",
				Thinking: thinking,
			}, runtime.AgentConfigInfo{})
			if thinking == "" {
				assert.NotContains(t, out, "Thinking:", "empty thinking label must omit the line")
				return
			}
			assert.Contains(t, out, "Thinking: "+ansi.Strip(toolcommon.ThinkingGaugeValue(thinking)))
		})
	}
}

// TestAgentDetailsDialog_RendersConfigSections verifies the inline compact
// config summaries — sub-agents, handoffs, skills and the fallback model —
// render with their counts, and that each section is omitted when its slice is
// empty.
func TestAgentDetailsDialog_RendersConfigSections(t *testing.T) {
	t.Parallel()

	withCfg := renderAgentDetails(runtime.AgentDetails{
		Name: "root", Provider: "openai", Model: "gpt-5.4", Thinking: "high",
	}, runtime.AgentConfigInfo{
		SubAgents: []string{"coder", "reviewer"},
		Handoffs:  []string{"planner"},
		Skills:    []string{"debugging", "refactor"},
		Fallbacks: []string{"anthropic/claude-opus-4-8"},
	})
	assert.Contains(t, withCfg, "Sub-agents (2): coder, reviewer")
	assert.Contains(t, withCfg, "Handoffs (1): planner")
	assert.Contains(t, withCfg, "Skills (2): debugging, refactor")
	assert.Contains(t, withCfg, "Fallback: anthropic/claude-opus-4-8")

	empty := renderAgentDetails(runtime.AgentDetails{Name: "root", Model: "gpt-5.4"}, runtime.AgentConfigInfo{})
	assert.NotContains(t, empty, "Sub-agents (", "no sub-agents section when none configured")
	assert.NotContains(t, empty, "Handoffs (", "no handoffs section when none configured")
	assert.NotContains(t, empty, "Skills (", "no skills section when none configured")
	assert.NotContains(t, empty, "Toolsets (", "no toolsets section when none configured")
	assert.NotContains(t, empty, "Limits:", "no limits line when none configured")
	assert.NotContains(t, empty, "Options:", "no options line when none configured")
	assert.NotContains(t, empty, "Fallback:", "no fallback line when none configured")
	assert.NotContains(t, empty, "current agent", "no live line when not current")
}

// TestAgentDetailsDialog_TitleUsesAgentAccentColor verifies the title is
// rendered in the agent's accent color (matching the sidebar), so two agents
// produce differently-colored titles. Not parallel: it mutates the global
// agent-order registry.
func TestAgentDetailsDialog_TitleUsesAgentAccentColor(t *testing.T) {
	t.Parallel()
	styles.SetAgentOrder([]string{"root", "helper"})
	t.Cleanup(func() { styles.SetAgentOrder(nil) })

	titleOf := func(name string) string {
		d := NewAgentDetailsDialog(runtime.AgentDetails{Name: name, Model: "gpt"}, runtime.AgentConfigInfo{}).(*agentDetailsDialog)
		return d.renderLines(80, 24)[0] // raw title line, with ANSI styling
	}

	root := titleOf("root")
	helper := titleOf("helper")

	assert.Equal(t, "root", strings.TrimSpace(ansi.Strip(root)))
	assert.Equal(t, "helper", strings.TrimSpace(ansi.Strip(helper)))
	assert.NotEqual(t, root, helper, "each agent's title is rendered in its own accent color")

	// The title matches DialogTitleStyle recolored with the agent's accent.
	want := RenderTitle("root", 80, styles.DialogTitleStyle.Foreground(styles.AgentAccentStyleFor("root").GetForeground()))
	assert.Equal(t, want, root)
}

func TestAgentDetailsDialog_RendersCommands(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{
		Name:     "root",
		Provider: "anthropic",
		Model:    "opus",
		Thinking: "high",
		Commands: types.Commands{
			"plan":     {Description: "Hand off to the planner", Agent: "planner"},
			"fix-lint": {Description: "Fix linting errors"},
		},
	}, runtime.AgentConfigInfo{})

	assert.Contains(t, out, "Commands (2)")
	assert.Contains(t, out, "/fix-lint")
	assert.Contains(t, out, "Fix linting errors")
	assert.Contains(t, out, "/plan")
	assert.Contains(t, out, "Hand off to the planner")
}

func TestAgentDetailsDialog_HarnessNoThinkingNoCommands(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{
		Name:        "slack",
		Description: "Slack agent",
		Model:       "claude-code",
	}, runtime.AgentConfigInfo{})

	assert.Contains(t, out, "slack")
	assert.Contains(t, out, "Model: claude-code")
	assert.NotContains(t, out, "Thinking:", "harness agent has no thinking line")
	assert.NotContains(t, out, "Commands", "no commands section without commands")
}

// TestAgentDetailsDialog_RendersToolsets verifies the Toolsets section: a
// status marker per toolset (● started / ○ stopped / ⚠ error), the name, kind
// and tool count, and the tools rendered as live names when started but as a
// "declared:" allow-list when stopped.
func TestAgentDetailsDialog_RendersToolsets(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{Name: "root", Model: "gpt-5.4"}, runtime.AgentConfigInfo{
		Toolsets: []runtime.ToolsetDetail{
			{Name: "filesystem", State: runtime.ToolsetStarted, Tools: []string{"read_file", "write_file"}},
			{Name: "git", State: runtime.ToolsetStopped, Tools: []string{"status", "commit"}},
			{Name: "search", Kind: "MCP", State: runtime.ToolsetError},
		},
	})

	assert.Contains(t, out, "Toolsets (3)")
	// Started: green marker, Built-in kind, live tool names (no "declared:").
	assert.Contains(t, out, "● filesystem (Built-in) · 2 tools")
	assert.Contains(t, out, "read_file, write_file")
	assert.NotContains(t, out, "declared: read_file")
	// Stopped: hollow marker, declared allow-list.
	assert.Contains(t, out, "○ git (Built-in) · 2 tools")
	assert.Contains(t, out, "declared: status, commit")
	// Error: warning marker, Kind label, no tools sub-line.
	assert.Contains(t, out, "⚠ search (MCP)")
}

// TestAgentDetailsDialog_RendersLiveStateLimitsOptionsSkills verifies the live
// "current agent" line, the limits line (only set values), the options line
// (only enabled flags), and the compact skills list.
func TestAgentDetailsDialog_RendersLiveStateLimitsOptionsSkills(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{Name: "root", Model: "gpt-5.4"}, runtime.AgentConfigInfo{
		IsCurrent:               true,
		MaxIterations:           50,
		NumHistoryItems:         40,
		MaxConsecutiveToolCalls: 5,
		Options:                 []string{"add-date", "redact-secrets"},
		Skills:                  []string{"debugging", "refactor"},
	})

	assert.Contains(t, out, "● current agent")
	assert.Contains(t, out, "Limits: max-iter 50 · history 40 · max-tool-calls 5")
	assert.Contains(t, out, "Options: add-date · redact-secrets")
	assert.Contains(t, out, "Skills (2): debugging, refactor")
}

// TestAgentDetailsDialog_LimitsOmitsUnsetValues verifies the limits line only
// lists the limits that are actually set (non-zero).
func TestAgentDetailsDialog_LimitsOmitsUnsetValues(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{Name: "root", Model: "gpt"}, runtime.AgentConfigInfo{
		NumHistoryItems: 40,
	})
	assert.Contains(t, out, "Limits: history 40")
	assert.NotContains(t, out, "max-iter", "unset max-iter is omitted")
	assert.NotContains(t, out, "max-tool-calls", "unset max-tool-calls is omitted")
}

// TestInlineList_NarrowWidth_NoLabelDuplication verifies that when contentWidth
// is narrower than the prefix string, the guard prevents the bold prefix from
// being concatenated with the (unstripped) first wrapped fragment, which would
// render the label text twice. At narrow width the whole output falls back to
// the muted style so the label appears exactly once. At normal width the bold
// prefix is applied correctly.
func TestInlineList_NarrowWidth_NoLabelDuplication(t *testing.T) {
	t.Parallel()

	d := &agentDetailsDialog{agent: runtime.AgentDetails{Name: "root"}}
	items := []string{"alpha", "beta", "gamma"}

	// contentWidth=10 forces the word wrapper to put just "Sub-agents" on the
	// first line (the full prefix "Sub-agents (3):" is 15 chars, so it splits).
	// Without the HasPrefix guard this produces:
	//   BoldStyle("Sub-agents (3):") + MutedStyle("Sub-agents") → label twice.
	narrow := d.inlineList(10, "Sub-agents", items)
	require.NotNil(t, narrow)
	narrowOut := ansi.Strip(strings.Join(narrow, "\n"))
	assert.Equal(t, 1, strings.Count(narrowOut, "Sub-agents"),
		"label must appear exactly once at narrow width; got: %q", narrowOut)

	// At normal width the first wrapped line starts with the full prefix, so
	// the bold prefix is applied and the content follows on the same line.
	wide := d.inlineList(60, "Sub-agents", items)
	wideOut := ansi.Strip(strings.Join(wide, "\n"))
	assert.Contains(t, wideOut, "Sub-agents (3): alpha, beta, gamma")
}

// TestAgentDetailsDialog_OmitsInstruction documents that the inspector never
// renders the agent's instruction/system prompt: neither AgentDetails nor
// AgentConfigInfo carries it, and no section heading exposes it.
func TestAgentDetailsDialog_OmitsInstruction(t *testing.T) {
	t.Parallel()

	out := renderAgentDetails(runtime.AgentDetails{
		Name:        "root",
		Description: "Routes work to sub-agents",
		Model:       "gpt-5.4",
	}, runtime.AgentConfigInfo{
		SubAgents: []string{"coder"},
	})

	assert.Contains(t, out, "Routes work to sub-agents", "description is shown")
	assert.NotContains(t, out, "Instruction", "the system prompt is never surfaced")
	assert.NotContains(t, out, "System prompt")
}
