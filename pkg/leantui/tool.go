package leantui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// toolView is the render state of a single tool call. The controller keeps one
// per in-flight call, updates it as argument and output deltas arrive, and
// commits its rendered form once the call completes.
type toolView struct {
	name        string // display name of the tool
	command     string // shell command, when the tool runs one
	argsSummary string // compact argument summary for non-shell tools
	output      string
	done        bool
	isError     bool
	elapsed     time.Duration
}

const maxToolOutputLines = 12

// renderTool renders a tool call in the pi style: a status bullet and bold
// title, the command or argument summary, a dimmed (and tail-truncated) output
// block, and a final "Took …" line once the call has completed.
func renderTool(t toolView, width int) []string {
	bullet := stWarning().Render("●")
	if t.done {
		if t.isError {
			bullet = stError().Render("●")
		} else {
			bullet = stSuccess().Render("●")
		}
	}

	out := []string{bullet + " " + stBold().Render(t.name)}

	switch {
	case t.command != "":
		for _, l := range wrapANSI("$ "+t.command, width-2) {
			out = append(out, "  "+stSecondary().Render(l))
		}
	case t.argsSummary != "":
		for _, l := range wrapANSI(t.argsSummary, width-2) {
			out = append(out, "  "+stMuted().Render(l))
		}
	}

	if strings.TrimSpace(t.output) != "" {
		out = append(out, renderToolOutput(t.output, width)...)
	}

	if t.done {
		footer := "Took " + formatDuration(t.elapsed)
		if t.isError {
			footer = stError().Render("Failed") + stMuted().Render(" · "+formatDuration(t.elapsed))
			out = append(out, "  "+footer)
		} else {
			out = append(out, "  "+stMuted().Render(footer))
		}
	}

	return out
}

func renderToolOutput(output string, width int) []string {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var out []string
	if len(lines) > maxToolOutputLines {
		hidden := len(lines) - maxToolOutputLines
		out = append(out, "  "+stMuted().Render(fmt.Sprintf("… (%d earlier lines)", hidden)))
		lines = lines[len(lines)-maxToolOutputLines:]
	}
	for _, l := range lines {
		for _, wl := range wrapANSI(l, width-2) {
			out = append(out, "  "+stMuted().Render(wl))
		}
	}
	return out
}

// describeToolCall extracts a shell command and/or a short argument summary
// from a tool call's JSON arguments for display.
func describeToolCall(argsJSON string) (command, summary string) {
	argsJSON = strings.TrimSpace(argsJSON)
	if argsJSON == "" {
		return "", ""
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		// Arguments are still streaming or not an object; show them raw.
		return "", truncate(strings.ReplaceAll(argsJSON, "\n", " "), 200)
	}

	for _, key := range []string{"command", "cmd", "script"} {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, ""
			}
		}
	}

	for _, key := range []string{"path", "file", "filename", "file_path", "query", "pattern", "url", "name"} {
		if v, ok := args[key]; ok {
			return "", fmt.Sprintf("%s: %v", key, v)
		}
	}

	// Fall back to a compact rendering of all scalar arguments.
	var parts []string
	for k, v := range args {
		switch v.(type) {
		case string, float64, bool, json.Number:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return "", truncate(strings.Join(parts, " "), 200)
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}
