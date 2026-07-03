package environment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequiredEnvError_NamesEverySecretSource(t *testing.T) {
	t.Parallel()

	err := &RequiredEnvError{Missing: []string{"ANTHROPIC_API_KEY", "GITHUB_PERSONAL_ACCESS_TOKEN"}}
	msg := err.Error()

	assert.Contains(t, msg, "environment variables must be set")
	assert.Contains(t, msg, " - ANTHROPIC_API_KEY")
	assert.Contains(t, msg, " - GITHUB_PERSONAL_ACCESS_TOKEN")

	// Every secret source comes with a concrete example using the first
	// missing variable.
	assert.Contains(t, msg, "export ANTHROPIC_API_KEY=<value>")
	assert.Contains(t, msg, "--env-from-file")
	assert.Contains(t, msg, `security add-generic-password -a "$USER" -s ANTHROPIC_API_KEY -w`)
	assert.Contains(t, msg, "pass insert ANTHROPIC_API_KEY")
	assert.Contains(t, msg, "Docker Desktop")
	assert.Contains(t, msg, "credential_helper")
	assert.Contains(t, msg, SecretsDocsURL)

	// The local-model alternative only applies to model credentials.
	assert.NotContains(t, msg, "dmr/ai/qwen3")
}

func TestRequiredEnvError_SuggestsLocalModelForModelCredentials(t *testing.T) {
	t.Parallel()

	err := &RequiredEnvError{
		Missing:                 []string{"OPENAI_API_KEY"},
		MissingModelCredentials: true,
	}
	msg := err.Error()

	assert.Contains(t, msg, "--model dmr/ai/qwen3")
	assert.Contains(t, msg, "Models already pulled in Docker Model Runner are detected automatically")
	assert.Contains(t, msg, "docker agent models")
}
