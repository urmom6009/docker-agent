package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/agent"
	"github.com/docker/docker-agent/pkg/chat"
	"github.com/docker/docker-agent/pkg/tools"
)

func TestGetMessagesMaxOldToolCallTokensDefaultDoesNotTruncate(t *testing.T) {
	t.Parallel()
	oldResult := strings.Repeat("old-result-", 15000)
	newResult := "new result"

	s := New()
	addToolExchange(s, "old", oldResult)
	addToolExchange(s, "new", newResult)

	messages := s.GetMessages(agent.New("test-agent", "test instruction"))

	assert.Equal(t, oldResult, toolResultContent(t, messages, "old"))
	assert.Equal(t, newResult, toolResultContent(t, messages, "new"))
}

func TestGetMessagesMaxOldToolCallTokensPositiveTruncatesOldContent(t *testing.T) {
	t.Parallel()
	oldResult := strings.Repeat("old-result-", 15000)
	newResult := "new result"

	s := New(WithMaxOldToolCallTokens(10))
	addToolExchange(s, "old", oldResult)
	addToolExchange(s, "new", newResult)

	messages := s.GetMessages(agent.New("test-agent", "test instruction"))

	assert.Equal(t, toolContentPlaceholder, toolResultContent(t, messages, "old"))
	assert.Equal(t, newResult, toolResultContent(t, messages, "new"))
}

func addToolExchange(s *Session, toolCallID, result string) {
	s.AddMessage(UserMessage("run " + toolCallID))
	s.AddMessage(NewAgentMessage("test-agent", &chat.Message{
		Role: chat.MessageRoleAssistant,
		ToolCalls: []tools.ToolCall{
			{
				ID: toolCallID,
				Function: tools.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"input":"` + toolCallID + `"}`,
				},
			},
		},
	}))
	s.AddMessage(NewAgentMessage("test-agent", &chat.Message{
		Role:       chat.MessageRoleTool,
		ToolCallID: toolCallID,
		Content:    result,
	}))
}

func toolResultContent(t *testing.T, messages []chat.Message, toolCallID string) string {
	t.Helper()

	for _, msg := range messages {
		if msg.Role == chat.MessageRoleTool && msg.ToolCallID == toolCallID {
			return msg.Content
		}
	}

	require.Failf(t, "tool result not found", "tool_call_id=%s", toolCallID)
	return ""
}
