package shell

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/tools"
)

func TestNewScript_Empty(t *testing.T) {
	t.Parallel()
	tool, err := NewScript(nil, nil)
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	assert.Empty(t, allTools)
}

func TestNewScript_ToolNoArg(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"get_ip": {
			Description: "Get public IP",
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	assert.Len(t, allTools, 1)

	schema, err := json.Marshal(allTools[0].Parameters)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	"type": "object",
	"properties": {}
}`, string(schema))
}

func TestNewScript_Tool(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"github_user_repos": {
			Description: "List GitHub repositories of the provided user",
			Args: map[string]any{
				"username": map[string]any{
					"description": "GitHub username to get the repository list for",
					"type":        "string",
				},
			},
			Required: []string{"username"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	assert.Len(t, allTools, 1)

	schema, err := json.Marshal(allTools[0].Parameters)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	"type": "object",
	"properties": {
		"username": {
			"description": "GitHub username to get the repository list for",
			"type": "string"
		}
	},
	"required": ["username"]
}`, string(schema))
}

func TestNewScript_Typo(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"docker_images": {
			Description: "List running Docker containers",
			Cmd:         "docker images $image",
			Args: map[string]any{
				"img": map[string]any{
					"description": "Docker image to list",
					"type":        "string",
				},
			},
			Required: []string{"img"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.Nil(t, tool)
	require.ErrorContains(t, err, "tool 'docker_images' uses undefined args: [image]")
}

func TestNewScript_MissingRequired(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"docker_images": {
			Description: "List running Docker containers",
			Cmd:         "docker images $image",
			Args: map[string]any{
				"image": map[string]any{
					"description": "Docker image to list",
					"type":        "string",
				},
			},
			Required: []string{"img"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.Nil(t, tool)
	require.ErrorContains(t, err, "tool 'docker_images' has required arg 'img' which is not defined in args")
}

func TestNewScript_NumberArg(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"repeat": {
			Description: "Repeat a message N times",
			Cmd:         "for i in $(seq 1 $count); do echo $message; done",
			Args: map[string]any{
				"message": map[string]any{
					"description": "Message to repeat",
					"type":        "string",
				},
				"count": map[string]any{
					"description": "Number of repetitions",
					"type":        "number",
				},
			},
			Required: []string{"message", "count"},
		},
	}

	tool, err := NewScript(shellTools, os.Environ())
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	require.Len(t, allTools, 1)

	// Simulate LLM sending a number argument (JSON numbers are float64)
	result, err := allTools[0].Handler(t.Context(), tools.ToolCall{
		Function: tools.FunctionCall{
			Arguments: `{"message": "hello", "count": 3}`,
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError, "unexpected error: %s", result.Output)
	assert.Equal(t, "hello\nhello\nhello\n", result.Output)
}

func TestScriptShellTool_DropsUndeclaredArgs(t *testing.T) {
	t.Parallel()
	// `env` lists the spawned process's full environment. With base env
	// set to an empty slice, the only entries should be those forwarded
	// from declared args.
	shellTools := map[string]latest.ScriptShellToolConfig{
		"echo_name": {
			Cmd: "env",
			Args: map[string]any{
				"name": map[string]any{
					"description": "who to greet",
					"type":        "string",
				},
			},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, []string{})
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	require.Len(t, allTools, 1)

	// The LLM hallucinates LD_PRELOAD alongside the declared `name`.
	// Only `name` should reach execve; LD_PRELOAD must be dropped.
	result, err := allTools[0].Handler(t.Context(), tools.ToolCall{
		Function: tools.FunctionCall{
			Arguments: `{"name": "alice", "LD_PRELOAD": "/tmp/evil.so"}`,
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError, "unexpected error: %s", result.Output)
	assert.Contains(t, result.Output, "name=alice")
	assert.NotContains(t, result.Output, "LD_PRELOAD")
}

func TestScriptShellTool_RejectsNULInValue(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"echo_name": {
			Cmd: "echo $name",
			Args: map[string]any{
				"name": map[string]any{
					"description": "who to greet",
					"type":        "string",
				},
			},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, []string{})
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	require.Len(t, allTools, 1)

	result, err := allTools[0].Handler(t.Context(), tools.ToolCall{
		Function: tools.FunctionCall{
			Arguments: "{\"name\": \"alice\\u0000extra\"}",
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Output, "NUL byte")
}

func TestScriptToolSet_InstructionsNoDescription(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"greet": {
			Cmd: "echo hello ${name}",
			Args: map[string]any{
				"name": map[string]any{
					"type": "string",
					// no "description" key — this must not panic
				},
			},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	// Must not panic even though description is absent.
	instructions := tool.Instructions()
	assert.Contains(t, instructions, "greet")
	assert.Contains(t, instructions, "`name`")
	assert.Contains(t, instructions, "(required)")
	// No trailing colon-space artefact when description is empty.
	assert.NotContains(t, instructions, "`name`: ")
}

func TestScriptToolSet_InstructionsNilArgDef(t *testing.T) {
	t.Parallel()
	// argDef is nil — must not panic.
	shellTools := map[string]latest.ScriptShellToolConfig{
		"greet": {
			Cmd:      "echo hello ${name}",
			Args:     map[string]any{"name": nil},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	instructions := tool.Instructions()
	assert.Contains(t, instructions, "`name`")
}

func TestScriptToolSet_InstructionsNonMapArgDef(t *testing.T) {
	t.Parallel()
	// argDef is a bare string (not a map) — must not panic.
	shellTools := map[string]latest.ScriptShellToolConfig{
		"greet": {
			Cmd:      "echo hello ${name}",
			Args:     map[string]any{"name": "string"},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	instructions := tool.Instructions()
	assert.Contains(t, instructions, "`name`")
}

func TestScriptToolSet_DeterministicOrdering(t *testing.T) {
	t.Parallel()
	// Three tools whose names sort as: alpha < beta < gamma.
	// Each tool has multiple args to also exercise arg-level sorting in Instructions().
	// Because Go map iteration is random, without explicit sorting the
	// returned order could be anything — this test catches regressions.
	shellTools := map[string]latest.ScriptShellToolConfig{
		"gamma": {
			Description: "Third tool",
			Cmd:         "echo $z $a",
			Args: map[string]any{
				"z": map[string]any{"type": "string", "description": "last arg"},
				"a": map[string]any{"type": "string", "description": "first arg"},
			},
			Required: []string{"z", "a"},
		},
		"alpha": {
			Description: "First tool",
			Cmd:         "echo $m $b",
			Args: map[string]any{
				"m": map[string]any{"type": "string", "description": "middle arg"},
				"b": map[string]any{"type": "string", "description": "earlier arg"},
			},
			Required: []string{"m", "b"},
		},
		"beta": {Description: "Second tool", Cmd: "echo beta"},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	// Tools() must return tools in alphabetical order.
	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	require.Len(t, allTools, 3)
	assert.Equal(t, "alpha", allTools[0].Name)
	assert.Equal(t, "beta", allTools[1].Name)
	assert.Equal(t, "gamma", allTools[2].Name)

	// Instructions() must list tool sections in alphabetical order.
	instructions := tool.Instructions()
	alphaPos := strings.Index(instructions, "### alpha")
	betaPos := strings.Index(instructions, "### beta")
	gammaPos := strings.Index(instructions, "### gamma")
	assert.Greater(t, alphaPos, -1, "alpha heading not found")
	assert.Greater(t, betaPos, -1, "beta heading not found")
	assert.Greater(t, gammaPos, -1, "gamma heading not found")
	assert.Less(t, alphaPos, betaPos, "alpha must appear before beta")
	assert.Less(t, betaPos, gammaPos, "beta must appear before gamma")

	// Instructions() must also list args in alphabetical order within each tool.
	// For "alpha": arg "b" must appear before arg "m".
	alphaSection := instructions[alphaPos:betaPos]
	bArgPos := strings.Index(alphaSection, "`b`")
	mArgPos := strings.Index(alphaSection, "`m`")
	assert.Greater(t, bArgPos, -1, "arg b not found in alpha section")
	assert.Greater(t, mArgPos, -1, "arg m not found in alpha section")
	assert.Less(t, bArgPos, mArgPos, "arg b must appear before arg m in alpha section")

	// For "gamma": arg "a" must appear before arg "z".
	gammaSection := instructions[gammaPos:]
	aArgPos := strings.Index(gammaSection, "`a`")
	zArgPos := strings.Index(gammaSection, "`z`")
	assert.Greater(t, aArgPos, -1, "arg a not found in gamma section")
	assert.Greater(t, zArgPos, -1, "arg z not found in gamma section")
	assert.Less(t, aArgPos, zArgPos, "arg a must appear before arg z in gamma section")
}

func TestNewScript_ArgWithoutType(t *testing.T) {
	t.Parallel()
	shellTools := map[string]latest.ScriptShellToolConfig{
		"greet": {
			Description: "Greet someone",
			Cmd:         "echo Hello $name",
			Args: map[string]any{
				"name": map[string]any{
					"description": "Name to greet",
				},
			},
			Required: []string{"name"},
		},
	}

	tool, err := NewScript(shellTools, nil)
	require.NoError(t, err)

	allTools, err := tool.Tools(t.Context())
	require.NoError(t, err)
	assert.Len(t, allTools, 1)

	schema, err := json.Marshal(allTools[0].Parameters)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	"type": "object",
	"properties": {
		"name": {
			"description": "Name to greet",
			"type": "string"
		}
	},
	"required": ["name"]
}`, string(schema))
}
