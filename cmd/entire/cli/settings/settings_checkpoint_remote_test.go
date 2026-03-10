package settings

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCheckpointRemote_NotConfigured(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{}
	assert.Empty(t, s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_EmptyStrategyOptions(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{},
	}
	assert.Empty(t, s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_SSHUrl(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{
			"checkpoint_remote": "git@github.com:org/checkpoints.git",
		},
	}
	assert.Equal(t, "git@github.com:org/checkpoints.git", s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_HTTPSUrl(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{
			"checkpoint_remote": "https://github.com/org/checkpoints.git",
		},
	}
	assert.Equal(t, "https://github.com/org/checkpoints.git", s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_EmptyString(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{
			"checkpoint_remote": "",
		},
	}
	assert.Empty(t, s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_WrongType(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{
			"checkpoint_remote": 42,
		},
	}
	assert.Empty(t, s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_JSONRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	entireDir := filepath.Join(tmpDir, ".entire")
	require.NoError(t, os.MkdirAll(entireDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".git"), 0o755))

	settingsJSON := `{
		"enabled": true,
		"strategy_options": {
			"checkpoint_remote": "git@github.com:org/checkpoints.git"
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(entireDir, "settings.json"), []byte(settingsJSON), 0o644))

	t.Chdir(tmpDir)

	s, err := Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:org/checkpoints.git", s.GetCheckpointRemote())
}

func TestGetCheckpointRemote_CoexistsWithPushSessions(t *testing.T) {
	t.Parallel()

	s := &EntireSettings{
		StrategyOptions: map[string]any{
			"push_sessions":     false,
			"checkpoint_remote": "git@github.com:org/checkpoints.git",
		},
	}
	assert.Equal(t, "git@github.com:org/checkpoints.git", s.GetCheckpointRemote())
	assert.True(t, s.IsPushSessionsDisabled())
}
