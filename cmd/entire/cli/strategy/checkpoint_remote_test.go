package strategy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureGitRemote_CreatesNew(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "f.txt", "init")
	testutil.GitAdd(t, tmpDir, "f.txt")
	testutil.GitCommit(t, tmpDir, "init")
	t.Chdir(tmpDir)

	ctx := t.Context()
	err := ensureGitRemote(ctx, "test-remote", "https://example.com/repo.git")
	require.NoError(t, err)

	// Verify the remote was created
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "test-remote")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "https://example.com/repo.git")
}

func TestEnsureGitRemote_UpdatesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "f.txt", "init")
	testutil.GitAdd(t, tmpDir, "f.txt")
	testutil.GitCommit(t, tmpDir, "init")
	t.Chdir(tmpDir)

	ctx := t.Context()

	// Create remote with initial URL
	err := ensureGitRemote(ctx, "test-remote", "https://old.example.com/repo.git")
	require.NoError(t, err)

	// Update to new URL
	err = ensureGitRemote(ctx, "test-remote", "https://new.example.com/repo.git")
	require.NoError(t, err)

	// Verify the URL was updated
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "test-remote")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), "https://new.example.com/repo.git")
}

func TestEnsureGitRemote_NoOpWhenSameURL(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "f.txt", "init")
	testutil.GitAdd(t, tmpDir, "f.txt")
	testutil.GitCommit(t, tmpDir, "init")
	t.Chdir(tmpDir)

	ctx := t.Context()
	url := "https://example.com/repo.git"

	err := ensureGitRemote(ctx, "test-remote", url)
	require.NoError(t, err)

	// Call again with same URL - should be a no-op
	err = ensureGitRemote(ctx, "test-remote", url)
	require.NoError(t, err)

	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "test-remote")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), url)
}

func TestIsRemoteReachable_UnreachableURL(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	t.Chdir(tmpDir)

	ctx := t.Context()
	// Use an invalid URL that will fail quickly
	assert.False(t, isRemoteReachable(ctx, "/nonexistent/path/to/repo.git"))
}

func TestIsRemoteReachable_ReachableLocalRepo(t *testing.T) {
	ctx := context.Background()

	// Create a bare repo that's reachable
	remoteDir := t.TempDir()
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", remoteDir)
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	t.Chdir(tmpDir)

	assert.True(t, isRemoteReachable(ctx, remoteDir))
}

func TestEnsureBranchFromRemote_CreatesLocalFromRemote(t *testing.T) {
	ctx := context.Background()

	// Set up a "remote" repo with a branch
	remoteDir := t.TempDir()
	testutil.InitRepo(t, remoteDir)
	testutil.WriteFile(t, remoteDir, "f.txt", "init")
	testutil.GitAdd(t, remoteDir, "f.txt")
	testutil.GitCommit(t, remoteDir, "init")

	// Get the default branch name before switching
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = remoteDir
	branchCmd.Env = testutil.GitIsolatedEnv()
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	defaultBranch := strings.TrimSpace(string(branchOut))

	// Create an orphan branch in the remote repo (simulating entire/checkpoints/v1)
	cmd := exec.CommandContext(ctx, "git", "checkout", "--orphan", "entire/checkpoints/v1")
	cmd.Dir = remoteDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	cmd = exec.CommandContext(ctx, "git", "rm", "-rf", ".")
	cmd.Dir = remoteDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	testutil.WriteFile(t, remoteDir, "metadata.json", `{"test": true}`)
	testutil.GitAdd(t, remoteDir, "metadata.json")

	cmd = exec.CommandContext(ctx, "git", "-c", "commit.gpgsign=false", "commit", "-m", "checkpoint data")
	cmd.Dir = remoteDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	// Go back to the default branch
	cmd = exec.CommandContext(ctx, "git", "checkout", defaultBranch)
	cmd.Dir = remoteDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	// Set up local repo with remote pointing to the "remote" repo
	localDir := t.TempDir()
	testutil.InitRepo(t, localDir)
	testutil.WriteFile(t, localDir, "f.txt", "init")
	testutil.GitAdd(t, localDir, "f.txt")
	testutil.GitCommit(t, localDir, "init")

	cmd = exec.CommandContext(ctx, "git", "remote", "add", "test-remote", remoteDir)
	cmd.Dir = localDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	t.Chdir(localDir)

	// Verify branch doesn't exist locally
	assert.False(t, testutil.BranchExists(t, localDir, "entire/checkpoints/v1"))

	// Fetch and set up the branch
	require.NoError(t, ensureBranchFromRemote(ctx, "test-remote", "entire/checkpoints/v1"))

	// Verify the branch now exists locally
	assert.True(t, testutil.BranchExists(t, localDir, "entire/checkpoints/v1"))
}

func TestEnsureBranchFromRemote_NoOpWhenBranchNotOnRemote(t *testing.T) {
	ctx := context.Background()

	// Set up a "remote" repo without the checkpoint branch
	remoteDir := t.TempDir()
	testutil.InitRepo(t, remoteDir)
	testutil.WriteFile(t, remoteDir, "f.txt", "init")
	testutil.GitAdd(t, remoteDir, "f.txt")
	testutil.GitCommit(t, remoteDir, "init")

	// Set up local repo
	localDir := t.TempDir()
	testutil.InitRepo(t, localDir)
	testutil.WriteFile(t, localDir, "f.txt", "init")
	testutil.GitAdd(t, localDir, "f.txt")
	testutil.GitCommit(t, localDir, "init")

	cmd := exec.CommandContext(ctx, "git", "remote", "add", "test-remote", remoteDir)
	cmd.Dir = localDir
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	t.Chdir(localDir)

	err := ensureBranchFromRemote(ctx, "test-remote", "entire/checkpoints/v1")
	require.NoError(t, err)

	// Branch should still not exist locally
	assert.False(t, testutil.BranchExists(t, localDir, "entire/checkpoints/v1"))
}

func TestResolveCheckpointRemote_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "f.txt", "init")
	testutil.GitAdd(t, tmpDir, "f.txt")
	testutil.GitCommit(t, tmpDir, "init")

	// Create settings without checkpoint_remote
	entireDir := filepath.Join(tmpDir, ".entire")
	require.NoError(t, os.MkdirAll(entireDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(entireDir, "settings.json"),
		[]byte(`{"enabled": true}`),
		0o644,
	))

	t.Chdir(tmpDir)

	ctx := t.Context()
	result := resolveCheckpointRemote(ctx, "origin")
	assert.Equal(t, "origin", result)
}

func TestResolveCheckpointRemote_UnreachableRemote(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.InitRepo(t, tmpDir)
	testutil.WriteFile(t, tmpDir, "f.txt", "init")
	testutil.GitAdd(t, tmpDir, "f.txt")
	testutil.GitCommit(t, tmpDir, "init")

	// Create settings with an unreachable checkpoint_remote
	entireDir := filepath.Join(tmpDir, ".entire")
	require.NoError(t, os.MkdirAll(entireDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(entireDir, "settings.json"),
		[]byte(`{"enabled": true, "strategy_options": {"checkpoint_remote": "/nonexistent/path/to/repo.git"}}`),
		0o644,
	))

	t.Chdir(tmpDir)

	ctx := t.Context()
	result := resolveCheckpointRemote(ctx, "origin")
	// Should fall back to default since remote is unreachable
	assert.Equal(t, "origin", result)
}

func TestResolveCheckpointRemote_ReachableRemote(t *testing.T) {
	ctx := context.Background()

	// Create a bare remote repo
	remoteDir := t.TempDir()
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", remoteDir)
	cmd.Env = testutil.GitIsolatedEnv()
	require.NoError(t, cmd.Run())

	// Create local repo with settings pointing to the bare remote
	localDir := t.TempDir()
	testutil.InitRepo(t, localDir)
	testutil.WriteFile(t, localDir, "f.txt", "init")
	testutil.GitAdd(t, localDir, "f.txt")
	testutil.GitCommit(t, localDir, "init")

	entireDir := filepath.Join(localDir, ".entire")
	require.NoError(t, os.MkdirAll(entireDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(entireDir, "settings.json"),
		[]byte(`{"enabled": true, "strategy_options": {"checkpoint_remote": "`+remoteDir+`"}}`),
		0o644,
	))

	t.Chdir(localDir)

	result := resolveCheckpointRemote(ctx, "origin")
	assert.Equal(t, checkpointRemoteName, result)

	// Verify the git remote was created
	getURL := exec.CommandContext(ctx, "git", "remote", "get-url", checkpointRemoteName)
	getURL.Dir = localDir
	output, err := getURL.Output()
	require.NoError(t, err)
	assert.Contains(t, string(output), remoteDir)
}
