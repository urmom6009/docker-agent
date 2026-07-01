package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafetyPolicy_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[SafetyPolicy]bool{
		"":                    true,
		SafetyPolicyUnsafe:    true,
		SafetyPolicySafer:     true,
		SafetyPolicyStrict:    true,
		SafetyPolicy("yolo"):  false,
		SafetyPolicy("Safer"): false, // case-sensitive on purpose
	}
	for in, want := range cases {
		assert.Equalf(t, want, in.IsValid(), "SafetyPolicy(%q).IsValid()", string(in))
	}
}

// WithSafetyPolicy(unsafe) must flip ToolsApproved=true so legacy
// branches on ToolsApproved (Decide's --yolo short-circuit) still fire.
// Safer/strict intentionally leave ToolsApproved alone.
func TestWithSafetyPolicy_UnsafeSyncsToolsApproved(t *testing.T) {
	t.Parallel()
	s := New(WithSafetyPolicy(SafetyPolicyUnsafe))
	assert.Equal(t, SafetyPolicyUnsafe, s.SafetyPolicy)
	assert.True(t, s.ToolsApproved)

	s = New(WithSafetyPolicy(SafetyPolicySafer))
	assert.Equal(t, SafetyPolicySafer, s.SafetyPolicy)
	assert.False(t, s.ToolsApproved)
}

// WithToolsApproved(true) must backfill SafetyPolicy=unsafe so hooks
// reading Input.SafetyPolicy see the correct value for legacy --yolo
// callers (gordon-Slack, MCP, eval) that haven't migrated.
func TestWithToolsApproved_BackfillsSafetyPolicy(t *testing.T) {
	t.Parallel()
	s := New(WithToolsApproved(true))
	assert.True(t, s.ToolsApproved)
	assert.Equal(t, SafetyPolicyUnsafe, s.SafetyPolicy)

	s = New(WithToolsApproved(false))
	assert.False(t, s.ToolsApproved)
	assert.Equal(t, SafetyPolicy(""), s.SafetyPolicy)
}

// Explicit WithSafetyPolicy after WithToolsApproved wins over the
// backfill (e.g. yolo + safer = "auto-approve except destructive").
func TestWithSafetyPolicy_ExplicitWinsOverToolsApproved(t *testing.T) {
	t.Parallel()
	s := New(
		WithToolsApproved(true),
		WithSafetyPolicy(SafetyPolicySafer),
	)
	assert.True(t, s.ToolsApproved)
	assert.Equal(t, SafetyPolicySafer, s.SafetyPolicy)
}
