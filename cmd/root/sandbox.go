package root

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/cli/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/docker/docker-agent/pkg/config"
	"github.com/docker/docker-agent/pkg/environment"
	"github.com/docker/docker-agent/pkg/paths"
	"github.com/docker/docker-agent/pkg/sandbox"
	"github.com/docker/docker-agent/pkg/sandbox/kit"
	"github.com/docker/docker-agent/pkg/skills"
)

// runInSandbox delegates the current command to a Docker sandbox.
// It ensures a sandbox exists (creating or recreating as needed), then
// executes docker agent inside it via the sandbox exec command.
func runInSandbox(ctx context.Context, cmd *cobra.Command, args []string, runConfig *config.RuntimeConfig, template string, preferSbx, noKit bool) error {
	if environment.InSandbox() {
		return fmt.Errorf("already running inside a Docker sandbox (VM %s)", os.Getenv("SANDBOX_VM_ID"))
	}

	backend := sandbox.NewBackend(preferSbx)

	if err := backend.CheckAvailable(ctx); err != nil {
		return err
	}

	var agentRef string
	if len(args) > 0 {
		agentRef = args[0]
	}

	configDir := paths.GetConfigDir()
	dockerAgentArgs := dockerAgentArgs(cmd, args, configDir)

	stopTokenWriter := sandbox.StartTokenWriterIfNeeded(ctx, configDir, runConfig.ModelsGateway)
	defer stopTokenWriter()

	// Resolve wd to an absolute path so that it matches the absolute
	// workspace paths returned by `docker sandbox ls --json`.
	wd, err := filepath.Abs(cmp.Or(runConfig.WorkingDir, "."))
	if err != nil {
		return fmt.Errorf("resolving workspace path: %w", err)
	}

	envProvider := environment.NewDefaultProvider()

	extras := []string{sandbox.ExtraWorkspace(wd, agentRef)}

	var kitResult *kit.Result
	if !noKit && agentRef != "" {
		kitResult, err = kit.Build(ctx, kit.Options{
			AgentRef:    agentRef,
			EnvProvider: envProvider,
			HostCwd:     wd,
			Workspace:   wd,
		})
		if err != nil {
			slog.WarnContext(ctx, "docker-agent kit build failed; continuing without kit", "error", err)
		} else {
			kitResult.PrintSummary(cmd.OutOrStdout())
			extras = append(extras, kitResult.HostDir)
			// We deliberately keep the kit on disk between runs:
			// the docker sandbox we reuse across runs holds a hard
			// reference to the kit's bind-mount path — deleting the
			// dir would leave the sandbox un-startable. The kit lives
			// in the cache dir keyed on a content hash, so the next
			// run for the same agent overwrites it in place; disk
			// usage is bounded by the number of distinct agents the
			// user has run.
		}
	}

	printModelsGateway(cmd.OutOrStdout(), runConfig.ModelsGateway)
	printToolInstallAllowance(cmd.OutOrStdout(), kitResult)

	name, err := backend.Ensure(ctx, wd, extras, template, configDir)
	if err != nil {
		return err
	}

	// Sandbox templates ship with a default-deny network proxy that
	// allows the major model providers (api.anthropic.com, api.openai.com,
	// ...) but blocks every *.docker.com host as well as every
	// package-registry / source-host the auto-installer reaches for.
	// Open the minimum: the configured Docker AI gateway when set, and
	// the package-host set only when the kit-build determined the
	// agent has at least one MCP / LSP toolset that may auto-install.
	needsToolInstall := kitResult != nil && kitResult.NeedsToolInstall
	allowSandboxHosts(ctx, backend, name, runConfig.ModelsGateway, needsToolInstall)

	// Resolve env vars the agent needs and forward them into the sandbox.
	// Docker Desktop proxies well-known API keys automatically; this handles
	// any additional vars (e.g. MCP tool secrets).
	envFlags, envVars := sandbox.EnvForAgent(ctx, agentRef, envProvider)

	// Forward the gateway by name so a URL with credentials never
	// shows up in the slog'd `docker sandbox exec` argv. We do not
	// forward DOCKER_TOKEN: inside the sandbox it must come only from
	// sandbox-tokens.json (kept fresh by StartTokenWriterIfNeeded).
	if gateway := runConfig.ModelsGateway; gateway != "" {
		envFlags = append(envFlags, "-e", envModelsGateway)
		envVars = append(envVars, envModelsGateway+"="+gateway)
	}

	// Point the in-sandbox resolvers at the staged kit. We use the
	// `-e KEY=VALUE` form so the value is set directly inside the
	// container; we deliberately do not append it to envVars (which
	// would set it on the host docker CLI process too — a path that
	// only makes sense inside the sandbox).
	if kitResult != nil {
		envFlags = append(envFlags, "-e", skills.KitDirEnv+"="+kit.MountPath)
	}

	dockerCmd := backend.BuildExecCmd(ctx, name, wd, dockerAgentArgs, envFlags, envVars)
	slog.DebugContext(ctx, "Executing in sandbox", "name", name, "args", dockerCmd.Args)

	if err := dockerCmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return cli.StatusError{StatusCode: exitErr.ExitCode()}
		}
		return fmt.Errorf("docker sandbox exec failed: %w", err)
	}

	return nil
}

func dockerAgentArgs(cmd *cobra.Command, args []string, configDir string) []string {
	skip := map[string]bool{
		"sandbox":    true,
		"sbx":        true,
		"config-dir": true,
		"no-kit":     true,
	}

	var dockerAgentArgs []string
	hasYolo := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if skip[f.Name] {
			return
		}

		if f.Name == "yolo" {
			hasYolo = true
		}

		if f.Value.Type() == "bool" {
			dockerAgentArgs = append(dockerAgentArgs, "--"+f.Name)
		} else {
			dockerAgentArgs = append(dockerAgentArgs, "--"+f.Name, f.Value.String())
		}
	})
	if !hasYolo {
		dockerAgentArgs = append(dockerAgentArgs, "--yolo")
	}

	dockerAgentArgs = append(dockerAgentArgs, args...)
	dockerAgentArgs = append(dockerAgentArgs, "--config-dir", configDir)

	return dockerAgentArgs
}

// autoInstallHosts is the set of hostnames the toolinstall package
// reaches for when fetching tools at runtime: the aqua registry data
// (raw.githubusercontent.com), the GitHub API (latest release
// resolution), GitHub releases themselves, the redirected release
// asset host (objects.githubusercontent.com), and the Go module
// proxy + checksum DB used by `go install`. We allowlist this whole
// set whenever a sandbox is launched with the kit pipeline so that
// auto-install — which is on by default for every lsp / mcp toolset
// — can actually fetch what it needs. Without this, missing tools
// (gopls, golangci-lint, ...) report a misleading "403 blocked by
// network policy" from go install / curl instead of installing.
var autoInstallHosts = []string{
	"github.com",
	"api.github.com",
	"raw.githubusercontent.com",
	"objects.githubusercontent.com",
	"codeload.github.com",
	"proxy.golang.org",
	"sum.golang.org",
	// `go install` downloads the Go toolchain from Google's blob
	// storage when a module's go.mod pins a newer Go than the one
	// already in the sandbox image.
	"storage.googleapis.com",
}

// allowSandboxHosts adds per-sandbox allow-network rules for every
// host the in-sandbox runtime is known to need: the configured
// models gateway (when set) and the package hosts the auto-installer
// reaches for (when needsToolInstall is true). The default sandbox
// proxy denies all of them; without this, the inner agent's first
// request returns a misleading "403 Blocked by network policy".
//
// Holes are punched only when the corresponding feature is in play:
//   - the gateway host is added only when gatewayURL is non-empty;
//   - the autoInstallHosts set is added only when needsToolInstall
//     is true (i.e. the kit build saw at least one MCP / LSP
//     toolset that might auto-install). Sandboxes that don't run
//     auto-install keep the strict default-deny.
//
// Best-effort: a malformed gateway URL or a backend that doesn't
// support per-sandbox policies is logged at debug level and the run
// proceeds. The user will then see a network-policy 403 from the
// inner and we surface that diagnostic verbatim.
func allowSandboxHosts(ctx context.Context, backend *sandbox.Backend, name, gatewayURL string, needsToolInstall bool) {
	var hosts []string

	if needsToolInstall {
		hosts = append(hosts, autoInstallHosts...)
	}

	if gatewayURL != "" {
		if h := gatewayHostPort(gatewayURL); h != "" {
			hosts = append(hosts, h)
		} else {
			slog.DebugContext(ctx, "Could not extract host from models-gateway URL; not allowlisting",
				"gateway", gatewayURL)
		}
	}

	if len(hosts) == 0 {
		return
	}
	if err := backend.AllowHosts(ctx, name, hosts); err != nil {
		slog.WarnContext(ctx, "Failed to allowlist sandbox hosts; the inner agent may see HTTP 403",
			"sandbox", name, "hosts", hosts, "error", err)
	}
}

// gatewayHostPort returns the "host" or "host:port" portion of a
// gateway URL, or "" if rawURL doesn't carry a usable authority.
//
// We accept both fully formed URLs (https://example.com:443/proxy),
// scheme-relative authorities (//example.com), and bare authorities
// (example.com:443) so users can configure the gateway either way.
// IPv6 brackets, ports, paths, queries and fragments are stripped
// from the returned value; malformed inputs (no host, scheme without
// host, opaque non-HTTP schemes, etc.) return "".
func gatewayHostPort(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	candidate := rawURL
	switch {
	case strings.HasPrefix(rawURL, "//"):
		// Protocol-relative authority — attach a fake scheme so url.Parse
		// recognises the host.
		candidate = "http:" + rawURL
	case hasURLScheme(rawURL):
		// Fully formed URL.
	case strings.Contains(rawURL, "://"):
		// Looks like a URL but the scheme part is bogus — reject.
		return ""
	case looksOpaque(rawURL):
		// e.g. "mailto:foo@example.com" — not a network endpoint.
		return ""
	default:
		// Bare authority. Strip trailing path / query / fragment up
		// front so url.Parse doesn't mistake "example.com:443/proxy"
		// for an opaque scheme.
		if i := strings.IndexAny(rawURL, "/?#"); i >= 0 {
			candidate = "http://" + rawURL[:i]
		} else {
			candidate = "http://" + rawURL
		}
	}

	u, err := url.Parse(candidate)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// hasURLScheme reports whether s looks like "scheme://..." with a
// non-empty scheme. We deliberately don't accept "scheme:opaque"
// (no //) because the gateway URL is always an HTTP(S) endpoint.
func hasURLScheme(s string) bool {
	i := strings.Index(s, "://")
	if i <= 0 {
		return false
	}
	return isSchemeChars(s[:i])
}

// looksOpaque reports whether s looks like an opaque-form URI
// ("scheme:rest", no //) such as "mailto:foo" or "data:...". These
// don't carry a network authority and must not be re-parsed as bare
// host strings.
func looksOpaque(s string) bool {
	i := strings.Index(s, ":")
	if i <= 0 {
		return false
	}
	// Reject anything with a port-looking right side, so values like
	// "example.com:443" are treated as bare authorities, not opaque.
	rhs := s[i+1:]
	if rhs != "" && rhs[0] >= '0' && rhs[0] <= '9' {
		return false
	}
	return isSchemeChars(s[:i])
}

// isSchemeChars reports whether s is a syntactically valid URL scheme
// (RFC 3986: ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ), starting with ALPHA).
func isSchemeChars(s string) bool {
	if s == "" {
		return false
	}
	first := s[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') {
		return false
	}
	for _, r := range s[1:] {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '+' || r == '-' || r == '.':
			continue
		default:
			return false
		}
	}
	return true
}

// displayGatewayURL returns a credential-safe rendering of a gateway
// URL: any user/password embedded in the authority is replaced with
// "***" so it never lands in stdout, slog output, or screenshots.
// Inputs that don't parse cleanly (or carry no userinfo) are returned
// unchanged.
//
// We rebuild the string by hand instead of calling u.String() because
// url.User on the latter URL-escapes the masking characters into
// %2A%2A%2A, which is technically correct but uglier than "***" in a
// human-facing line.
func displayGatewayURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}

	var b strings.Builder
	if u.Scheme != "" {
		b.WriteString(u.Scheme)
		b.WriteString("://")
	}
	b.WriteString("***@")
	b.WriteString(u.Host)
	b.WriteString(u.RequestURI())
	if u.Fragment != "" {
		b.WriteByte('#')
		b.WriteString(u.Fragment)
	}
	return b.String()
}

// printModelsGateway prints which AI gateway (if any) the inner agent
// will route model requests through. Surfacing this between the kit
// summary and the sandbox-create line makes it obvious — at a glance
// — whether an "HTTP 403" the user might see later originated from
// the proxy's network policy (gateway host not yet allow-listed) or
// from the gateway server itself.
//
// An unset gateway means we did not configure routing through one;
// the inner agent will fall back to whatever the docker-agent process
// resolves from its own environment (DOCKER_TOKEN, OS env vars,
// Docker Model Runner, etc.).
func printModelsGateway(w io.Writer, gateway string) {
	if gateway == "" {
		fmt.Fprintln(w, "Models gateway: none configured")
		return
	}
	display := displayGatewayURL(gateway)
	host := gatewayHostPort(gateway)
	if host == "" || host == gateway {
		fmt.Fprintf(w, "Models gateway: %s\n", display)
		return
	}
	fmt.Fprintf(w, "Models gateway: %s (allowlisting %s in the sandbox proxy)\n", display, host)
}

// printToolInstallAllowance prints a single line announcing whether
// the package-host allowlist was opened for this sandbox, and why.
// We surface this so the user sees what holes were punched in the
// default-deny network policy. Silent when the kit isn't built or
// when no auto-installable toolset was detected.
func printToolInstallAllowance(w io.Writer, kitResult *kit.Result) {
	if kitResult == nil || !kitResult.NeedsToolInstall {
		return
	}
	fmt.Fprintf(w, "Tool install: agent has at least one MCP/LSP toolset, allowlisting %d package hosts in the sandbox proxy\n",
		len(autoInstallHosts))
}
