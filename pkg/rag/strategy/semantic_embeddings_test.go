package strategy

import (
	"strings"
	"testing"
)

func TestFormatASTContext(t *testing.T) {
	t.Parallel()
	metadata := map[string]string{
		"symbol_name":        "Init",
		"symbol_kind":        "method",
		"signature":          "func (p *chatPage) Init() tea.Cmd",
		"doc":                "Init prepares the chat page.",
		"package":            "chat",
		"additional_symbols": "InitSidebar",
		"custom_note":        "requires session state",
	}

	formatted := formatASTContext(metadata)
	if formatted == "" {
		t.Fatalf("expected formatted AST context but got empty string")
	}

	if !strings.Contains(formatted, "AST context:") {
		t.Fatalf("expected prefix 'AST context:' in %q", formatted)
	}

	if !strings.Contains(formatted, "- Symbol: Init") {
		t.Fatalf("expected symbol line in %q", formatted)
	}

	if !strings.Contains(formatted, "- Custom Note: requires session state") {
		t.Fatalf("expected custom key to be humanized in %q", formatted)
	}

	if got := formatASTContext(nil); got != "" {
		t.Fatalf("expected empty string for nil metadata, got %q", got)
	}
}
