package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClone_ChangeWorkingDir(t *testing.T) {
	t.Parallel()
	original := &RuntimeConfig{
		Config: Config{
			EnvFiles:       []string{"file1.env", "file2.env"},
			ModelsGateway:  "http://models.gateway",
			GlobalCodeMode: true,
			WorkingDir:     "/app",
		},
	}

	clone := original.Clone()
	original.WorkingDir = "/newapp"
	clone.WorkingDir = "/cloneapp"

	assert.Equal(t, "/newapp", original.WorkingDir)
	assert.Equal(t, "/cloneapp", clone.WorkingDir)
}
