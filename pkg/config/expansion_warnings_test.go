package config

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/docker/docker-agent/pkg/config/latest"
	"github.com/docker/docker-agent/pkg/config/types"
)

func captureWarnings(t *testing.T, fn func(ctx context.Context)) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	fn(t.Context())
	return buf.String()
}

func TestWarnExpansionMismatches_ShellSyntaxInJSField(t *testing.T) {
	cfg := &latest.Config{
		Agents: []latest.AgentConfig{{
			Name:           "root",
			Description:    "Hi ${USER}",
			WelcomeMessage: "Welcome ${HOME}",
			Commands: types.Commands{
				"deploy": types.Command{Instruction: "Deploy ${PROJECT}"},
			},
		}},
	}

	out := captureWarnings(t, func(ctx context.Context) {
		warnExpansionMismatches(ctx, cfg)
	})

	assert.Contains(t, out, "USER")
	assert.Contains(t, out, "HOME")
	assert.Contains(t, out, "PROJECT")
	assert.Contains(t, out, "shell-style")
}

func TestWarnExpansionMismatches_JSEnvInPathField(t *testing.T) {
	cfg := &latest.Config{
		Agents: []latest.AgentConfig{{
			Name: "root",
			Toolsets: []latest.Toolset{{
				Type:       "mcp",
				Command:    "x",
				WorkingDir: "${env.HOME}/work",
			}, {
				Type: "memory",
				Path: "${env.MEM_DIR}/db",
			}},
		}},
	}

	out := captureWarnings(t, func(ctx context.Context) {
		warnExpansionMismatches(ctx, cfg)
	})

	assert.Contains(t, out, "JS-style")
	assert.Contains(t, out, "working_dir")
	assert.Contains(t, out, "path")
}

func TestWarnExpansionMismatches_NoFalsePositives(t *testing.T) {
	cfg := &latest.Config{
		Agents: []latest.AgentConfig{{
			Name:           "root",
			Description:    "Hello ${env.USER || 'guest'}",
			WelcomeMessage: "",
			Instruction:    "Use ${env.PROJECT} carefully",
			Commands: types.Commands{
				"greet": types.Command{Instruction: "Hello ${env.USER}"},
			},
			Toolsets: []latest.Toolset{{
				Type:       "mcp",
				Command:    "x",
				WorkingDir: "~/work/${PROJECT}",
				Headers: map[string]string{
					"Authorization": "Bearer ${env.TOKEN}",
				},
				Env: map[string]string{
					"FOO": "${env.BAR}",
				},
			}, {
				Type: "memory",
				Path: "$HOME/db",
			}},
		}},
	}

	out := captureWarnings(t, func(ctx context.Context) {
		warnExpansionMismatches(ctx, cfg)
	})

	if strings.Contains(out, "WARN") {
		t.Errorf("unexpected warnings emitted: %s", out)
	}
}

func TestWarnExpansionMismatches_HeaderMixedWithEnv(t *testing.T) {
	// When a value already references ${env.X}, we treat any other ${...} as
	// intentional JS so we don't second-guess legitimate template literals.
	cfg := &latest.Config{
		Agents: []latest.AgentConfig{{
			Name: "root",
			Toolsets: []latest.Toolset{{
				Type: "openapi",
				URL:  "https://x",
				Headers: map[string]string{
					"X": "${env.A} ${B}",
				},
			}},
		}},
	}

	out := captureWarnings(t, func(ctx context.Context) {
		warnExpansionMismatches(ctx, cfg)
	})

	assert.NotContains(t, out, "WARN")
}
