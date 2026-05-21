package config

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/docker-agent/pkg/config/latest"
)

// shellEnvVarRef matches a `${IDENT}` where IDENT looks like a shell environment
// variable name (uppercase letters, digits, underscores, leading non-digit).
// Used to flag shell-style references that appear in JS-template fields, where
// they will be silently passed through as literals instead of expanded.
var shellEnvVarRef = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// jsEnvRef matches the JS-template form `${env.X}`.
var jsEnvRef = regexp.MustCompile(`\$\{\s*env\.[A-Za-z_][A-Za-z0-9_]*`)

// warnExpansionMismatches scans a loaded config for fields whose contents use
// the wrong variable-expansion syntax for that field. Two incompatible
// syntaxes coexist today (issue #2615): JS template literals (`${env.X}`) for
// prompt/header fields and shell-style (`$VAR`/`${VAR}`) for path fields.
// Mixing them up currently fails silently; we emit warnings to make the
// problem visible without changing runtime behavior.
func warnExpansionMismatches(ctx context.Context, cfg *latest.Config) {
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		warnJSField(ctx, a.Name, "description", a.Description)
		warnJSField(ctx, a.Name, "welcome_message", a.WelcomeMessage)
		warnJSField(ctx, a.Name, "instruction", a.Instruction)

		for name, cmd := range a.Commands {
			warnJSField(ctx, a.Name, "commands."+name+".instruction", cmd.Instruction)
			warnJSField(ctx, a.Name, "commands."+name+".description", cmd.Description)
		}

		for j := range a.Toolsets {
			t := &a.Toolsets[j]
			loc := agentToolsetLocation(a.Name, t, j)

			warnJSField(ctx, loc, "instruction", t.Instruction)
			for k, v := range t.Headers {
				warnJSField(ctx, loc, "headers."+k, v)
			}
			for k, v := range t.Remote.Headers {
				warnJSField(ctx, loc, "remote.headers."+k, v)
			}
			for k, v := range t.Env {
				warnJSField(ctx, loc, "env."+k, v)
			}

			warnPathField(ctx, loc, "working_dir", t.WorkingDir)
			warnPathField(ctx, loc, "path", t.Path)
		}
	}
}

func agentToolsetLocation(agentName string, t *latest.Toolset, idx int) string {
	kind := t.Type
	if kind == "" {
		kind = "?"
	}
	return "agent " + agentName + " toolset[" + strconv.Itoa(idx) + "] (" + kind + ")"
}

// warnJSField warns when a JS-template field contains a `${VAR}` reference
// that looks like a shell variable name (uppercase identifier) and no
// `${env.X}` appears in the same value. Such references are kept literal at
// runtime instead of being expanded.
func warnJSField(ctx context.Context, loc, field, value string) {
	if value == "" {
		return
	}
	if jsEnvRef.MatchString(value) {
		// Has a real ${env.X}; assume any other ${...} is intentional JS.
		return
	}
	for _, m := range shellEnvVarRef.FindAllStringSubmatch(value, -1) {
		slog.WarnContext(ctx,
			"shell-style ${VAR} in JS-expanded field will not be substituted; use ${env.VAR}",
			"location", loc,
			"field", field,
			"variable", m[1],
			"see", "https://github.com/docker/docker-agent/issues/2615",
		)
	}
}

// warnPathField warns when a path field contains a `${env.X}` reference,
// which is JS-template syntax that path expansion (os.ExpandEnv + ~) does not
// recognize.
func warnPathField(ctx context.Context, loc, field, value string) {
	if value == "" {
		return
	}
	if !strings.Contains(value, "${env.") {
		return
	}
	slog.WarnContext(ctx,
		"JS-style ${env.X} in path field will not be substituted; use ${X} or $X",
		"location", loc,
		"field", field,
		"value", value,
		"see", "https://github.com/docker/docker-agent/issues/2615",
	)
}
