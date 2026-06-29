package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildSkillSystemMessage pins the skill-fork system prompt: skill
// name present, next-user-message directive present, attached files
// listed when provided, no task-delegation boilerplate.
func TestBuildSkillSystemMessage(t *testing.T) {
	t.Parallel()
	t.Run("no attachments", func(t *testing.T) {
		prepared := &PreparedSkillFork{SkillName: "commit"}
		msg := BuildSkillSystemMessage(prepared, nil)
		assert.Contains(t, msg, `"commit" skill`)
		assert.Contains(t, msg, "next user message")
		assert.NotContains(t, msg, "<attached_files>")
		assert.NotContains(t, msg, "<task>", "must not reuse buildTaskSystemMessage boilerplate")
		assert.NotContains(t, msg, "team of agents", "must not impersonate transfer_task")
	})

	t.Run("with attached files", func(t *testing.T) {
		prepared := &PreparedSkillFork{SkillName: "review"}
		msg := BuildSkillSystemMessage(prepared, []string{"/abs/foo.go", "/abs/bar.md"})
		assert.Contains(t, msg, "<attached_files>")
		assert.Contains(t, msg, "- /abs/foo.go")
		assert.Contains(t, msg, "- /abs/bar.md")
		assert.Contains(t, msg, "</attached_files>")
	})
}

// TestBuildSkillUserMessage_StripsFrontmatter pins the user message
// shape: <skill> envelope, frontmatter stripped, body verbatim,
// User's request: header for non-empty Task.
func TestBuildSkillUserMessage_StripsFrontmatter(t *testing.T) {
	t.Parallel()
	prepared := &PreparedSkillFork{
		SkillName: "commit",
		Task:      "fix the typo",
		Content: "---\n" +
			"name: commit\n" +
			"description: Commit local changes\n" +
			"context: fork\n" +
			"---\n" +
			"\n" +
			"# Commit\n" +
			"Use `git commit -q -m \"<subject>\"` for the common single-line case.\n",
	}

	msg := BuildSkillUserMessage(prepared)

	assert.True(t, strings.HasPrefix(msg, "Use the following skill."))
	assert.Contains(t, msg, "User's request: fix the typo")
	assert.Contains(t, msg, `<skill name="commit">`)
	assert.Contains(t, msg, "# Commit")
	assert.Contains(t, msg, `git commit -q -m "<subject>"`, "backtick markdown passes through untouched")
	assert.NotContains(t, msg, "context: fork", "frontmatter must not leak")
	assert.NotContains(t, msg, "name: commit\n", "frontmatter must not leak")
}

// TestBuildSkillUserMessage_NoTask: empty Task drops the User's request: header.
func TestBuildSkillUserMessage_NoTask(t *testing.T) {
	t.Parallel()
	prepared := &PreparedSkillFork{
		SkillName: "review",
		Content:   "---\nname: review\ndescription: x\n---\nReview the code.\n",
	}

	msg := BuildSkillUserMessage(prepared)

	assert.NotContains(t, msg, "User's request:")
	assert.Contains(t, msg, "Review the code.")
}

// TestStripFrontmatter covers no-fence, well-formed, unterminated, and
// leading-blank-line cases.
func TestStripFrontmatter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no frontmatter",
			in:   "# Skill\nbody\n",
			want: "# Skill\nbody\n",
		},
		{
			name: "well-formed frontmatter",
			in:   "---\nname: x\n---\nbody\n",
			want: "body\n",
		},
		{
			name: "unterminated frontmatter is left alone",
			in:   "---\nname: x\nbody without closing fence\n",
			want: "---\nname: x\nbody without closing fence\n",
		},
		{
			name: "frontmatter with leading newline preserved",
			in:   "---\nname: x\n---\n\n# Heading\n",
			want: "\n# Heading\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stripFrontmatter(tc.in))
		})
	}
}
