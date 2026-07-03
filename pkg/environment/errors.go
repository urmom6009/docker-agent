package environment

import (
	"fmt"
	"strings"
)

// SecretsDocsURL is the documentation page describing every built-in secret
// source and how to configure it.
const SecretsDocsURL = "https://docs.docker.com/ai/docker-agent/guides/secrets/"

type RequiredEnvError struct {
	Missing []string

	// MissingModelCredentials reports whether at least one missing variable is
	// a model-provider credential, in which case the error also suggests
	// running a local model instead of configuring an API key.
	MissingModelCredentials bool
}

var _ error = &RequiredEnvError{}

func (e *RequiredEnvError) Error() string {
	var msg strings.Builder

	fmt.Fprintln(&msg, "The following environment variables must be set:")
	for _, v := range e.Missing {
		fmt.Fprintf(&msg, " - %s\n", v)
	}

	example := "OPENAI_API_KEY"
	if len(e.Missing) > 0 {
		example = e.Missing[0]
	}
	msg.WriteString("\n")
	msg.WriteString(SecretSourcesHelp(example))

	if e.MissingModelCredentials {
		msg.WriteString("\nNo API key? Run a local model instead: docker agent run --model dmr/ai/qwen3 ...\nModels already pulled in Docker Model Runner are detected automatically by the default `auto` model.\n(`docker agent models` lists the models available to you)\n")
	}

	return msg.String()
}

// SecretSourcesHelp returns guidance naming every built-in secret source with
// a one-line example for the given variable, plus a link to the docs. It is
// shared by the errors that report missing credentials so the advice never
// drifts between them.
func SecretSourcesHelp(exampleVar string) string {
	var b strings.Builder
	b.WriteString("Provide them using any of these sources:\n")
	fmt.Fprintf(&b, " - Shell environment:  export %s=<value>\n", exampleVar)
	b.WriteString(" - Env file:           docker agent run --env-from-file <file> ...\n")
	fmt.Fprintf(&b, " - macOS Keychain:     security add-generic-password -a \"$USER\" -s %s -w\n", exampleVar)
	fmt.Fprintf(&b, " - pass:               pass insert %s\n", exampleVar)
	b.WriteString(" - Docker Desktop:     keys stored in Docker Desktop are picked up when you are signed in\n")
	b.WriteString(" - Credential helper:  set `credential_helper` in ~/.config/cagent/config.yaml\n")
	fmt.Fprintf(&b, "\nSee %s for details.\n", SecretsDocsURL)
	return b.String()
}
