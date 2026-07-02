package path

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		input    string
		envSetup map[string]string
		expected string
	}{
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "tilde with subpath",
			input:    "~/data/memory.db",
			expected: filepath.Join(home, "data", "memory.db"),
		},
		{
			name:     "env var",
			input:    "${HOME}/.data/memory.db",
			expected: filepath.Join(home, ".data", "memory.db"),
		},
		{
			name:     "custom env var",
			input:    "${MY_TEST_DATA_DIR}/memory.db",
			envSetup: map[string]string{"MY_TEST_DATA_DIR": "/tmp/testdata"},
			expected: "/tmp/testdata/memory.db",
		},
		{
			name:     "absolute path unchanged",
			input:    "/absolute/path/memory.db",
			expected: "/absolute/path/memory.db",
		},
		{
			name:     "relative path unchanged",
			input:    "relative/path/memory.db",
			expected: "relative/path/memory.db",
		},
		{
			name:     "tilde and env var combined",
			input:    "~/${MY_TEST_SUBDIR}/memory.db",
			envSetup: map[string]string{"MY_TEST_SUBDIR": "data"},
			expected: filepath.Join(home, "data", "memory.db"),
		},
		{
			name:     "js env ref",
			input:    "${env.HOME}/.data/memory.db",
			expected: filepath.Join(home, ".data", "memory.db"),
		},
		{
			name:     "js env ref custom var",
			input:    "${env.MY_TEST_DATA_DIR}/memory.db",
			envSetup: map[string]string{"MY_TEST_DATA_DIR": "/tmp/testdata"},
			expected: "/tmp/testdata/memory.db",
		},
		{
			name:     "js env ref with surrounding whitespace",
			input:    "${ env.MY_TEST_DATA_DIR }/memory.db",
			envSetup: map[string]string{"MY_TEST_DATA_DIR": "/tmp/testdata"},
			expected: "/tmp/testdata/memory.db",
		},
		{
			name:     "tilde and js env ref combined",
			input:    "~/${env.MY_TEST_SUBDIR}/memory.db",
			envSetup: map[string]string{"MY_TEST_SUBDIR": "data"},
			expected: filepath.Join(home, "data", "memory.db"),
		},
		{
			name:     "shell and js env refs mixed",
			input:    "${MY_TEST_DATA_DIR}/${env.MY_TEST_SUBDIR}/memory.db",
			envSetup: map[string]string{"MY_TEST_DATA_DIR": "/tmp/testdata", "MY_TEST_SUBDIR": "data"},
			expected: "/tmp/testdata/data/memory.db",
		},
		{
			name:     "undefined js env ref expands to empty",
			input:    "/base/${env.MY_TEST_UNDEFINED}/memory.db",
			expected: "/base//memory.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envSetup {
				t.Setenv(k, v)
			}
			result := ExpandPath(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExpandEnvRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envSetup map[string]string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "strict js env ref",
			input:    "Bearer ${env.MY_TEST_TOKEN}",
			envSetup: map[string]string{"MY_TEST_TOKEN": "secret"},
			expected: "Bearer secret",
		},
		{
			name:     "shell forms are kept literal",
			input:    "$MY_TEST_TOKEN and ${MY_TEST_TOKEN}",
			envSetup: map[string]string{"MY_TEST_TOKEN": "secret"},
			expected: "$MY_TEST_TOKEN and ${MY_TEST_TOKEN}",
		},
		{
			name:     "literal dollar untouched",
			input:    "pa$$word${",
			expected: "pa$$word${",
		},
		{
			name:     "rich js expression untouched",
			input:    "${env.MY_TEST_TOKEN || 'fallback'}",
			envSetup: map[string]string{"MY_TEST_TOKEN": "secret"},
			expected: "${env.MY_TEST_TOKEN || 'fallback'}",
		},
		{
			name:     "undefined var expands to empty",
			input:    "x${env.MY_TEST_UNDEFINED}y",
			expected: "xy",
		},
		{
			name:     "multiple refs",
			input:    "${env.MY_TEST_A}:${env.MY_TEST_B}",
			envSetup: map[string]string{"MY_TEST_A": "1", "MY_TEST_B": "2"},
			expected: "1:2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envSetup {
				t.Setenv(k, v)
			}
			result := ExpandEnvRefs(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandEnvRefs(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExpandWorkingDir(t *testing.T) {
	t.Setenv("MY_TEST_WD", "/tmp/wd")

	// Expands like ExpandPath.
	if got := ExpandWorkingDir("test", "${env.MY_TEST_WD}"); got != "/tmp/wd" {
		t.Errorf("got %q, want /tmp/wd", got)
	}
	// Empty input stays empty (no warning path).
	if got := ExpandWorkingDir("test", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	// Unset variable expands to empty (warning logged as side effect).
	if got := ExpandWorkingDir("test", "${env.MY_TEST_WD_UNSET}"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
