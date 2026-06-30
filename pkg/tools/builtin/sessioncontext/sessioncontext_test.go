package sessioncontext

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/tools"
)

func TestToolsMetadata(t *testing.T) {
	ts := New()
	defs, err := ts.Tools(t.Context())
	require.NoError(t, err)

	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
		assert.True(t, d.Annotations.ReadOnlyHint, "%s should be read-only", d.Name)
	}
	assert.ElementsMatch(t, []string{ToolNameListSessions, ToolNameReadSession}, names)
}

func TestInstructionsMentionBothTools(t *testing.T) {
	instr := New().Instructions()
	assert.Contains(t, instr, ToolNameListSessions)
	assert.Contains(t, instr, ToolNameReadSession)
}

func TestClampLimit(t *testing.T) {
	assert.Equal(t, DefaultListLimit, ClampLimit(0))
	assert.Equal(t, DefaultListLimit, ClampLimit(-5))
	assert.Equal(t, 7, ClampLimit(7))
	assert.Equal(t, MaxListLimit, ClampLimit(MaxListLimit+1))
	assert.Equal(t, MaxListLimit, ClampLimit(10_000))
}

func userMsg(content string) chat.Message {
	return chat.Message{Role: chat.MessageRoleUser, Content: content}
}

func assistantMsg(content string) chat.Message {
	return chat.Message{Role: chat.MessageRoleAssistant, Content: content}
}

func TestRenderTranscriptHeaderAndBody(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out := RenderTranscript(Header{
		ID:          "abc123",
		Title:       "Investigate flaky test",
		CreatedAt:   created,
		NumMessages: 2,
	}, []chat.Message{
		userMsg("why does TestFoo flake?"),
		assistantMsg("it depends on wall-clock time"),
	}, 0)

	assert.Contains(t, out, "# Session abc123 — Investigate flaky test")
	assert.Contains(t, out, "Created: 2026-01-02T03:04:05Z")
	assert.Contains(t, out, "Messages: 2")
	assert.Contains(t, out, "## user")
	assert.Contains(t, out, "why does TestFoo flake?")
	assert.Contains(t, out, "## assistant")
	assert.Contains(t, out, "it depends on wall-clock time")
	assert.NotContains(t, out, "omitted")
}

func TestRenderTranscriptUntitled(t *testing.T) {
	out := RenderTranscript(Header{ID: "x"}, []chat.Message{userMsg("hi")}, 0)
	assert.Contains(t, out, "# Session x — (untitled)")
	// A zero CreatedAt is omitted rather than printing a year-1 timestamp.
	assert.NotContains(t, out, "Created:")
}

func TestRenderTranscriptRendersToolCalls(t *testing.T) {
	msg := chat.Message{
		Role: chat.MessageRoleAssistant,
		ToolCalls: []tools.ToolCall{
			{Function: tools.FunctionCall{Name: "read_file", Arguments: `{"path":"main.go"}`}},
		},
	}
	out := RenderTranscript(Header{ID: "x"}, []chat.Message{msg}, 0)
	assert.Contains(t, out, "→ called tool `read_file` with {\"path\":\"main.go\"}")
}

func TestRenderTranscriptSkipsEmptyMessages(t *testing.T) {
	out := RenderTranscript(Header{ID: "x"}, []chat.Message{
		{Role: chat.MessageRoleAssistant, Content: "   "},
		userMsg("real content"),
	}, 0)
	// The whitespace-only assistant message produces no block.
	assert.Equal(t, 1, strings.Count(out, "## "))
	assert.Contains(t, out, "real content")
}

func TestRenderTranscriptMultiContentFallback(t *testing.T) {
	msg := chat.Message{
		Role: chat.MessageRoleUser,
		MultiContent: []chat.MessagePart{
			{Type: chat.MessagePartTypeText, Text: "part one"},
			{Type: chat.MessagePartTypeText, Text: "part two"},
		},
	}
	out := RenderTranscript(Header{ID: "x"}, []chat.Message{msg}, 0)
	assert.Contains(t, out, "part one")
	assert.Contains(t, out, "part two")
}

func TestRenderTranscriptTruncatesOldestFirst(t *testing.T) {
	msgs := []chat.Message{
		userMsg("OLDEST oldest oldest " + strings.Repeat("a", 200)),
		assistantMsg("MIDDLE middle middle " + strings.Repeat("b", 200)),
		userMsg("NEWEST newest newest " + strings.Repeat("c", 200)),
	}
	// Budget large enough for the header + roughly one message block.
	out := RenderTranscript(Header{ID: "x", NumMessages: len(msgs)}, msgs, 350)

	assert.Contains(t, out, "omitted")
	assert.Contains(t, out, "NEWEST", "the most recent message must be kept")
	assert.NotContains(t, out, "OLDEST", "the oldest message must be dropped first")
	assert.LessOrEqual(t, len(out), 400, "output should respect the char budget")
}

func TestRenderTranscriptKeepsAtLeastOneBlock(t *testing.T) {
	// Even with an impossibly small budget, the single newest block is kept so
	// read_session never returns a header with no conversation.
	out := RenderTranscript(Header{ID: "x", NumMessages: 1}, []chat.Message{
		userMsg("the only message " + strings.Repeat("z", 500)),
	}, 10)
	assert.Contains(t, out, "the only message")
}
