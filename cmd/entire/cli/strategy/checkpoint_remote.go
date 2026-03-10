package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/settings"

	"github.com/go-git/go-git/v6/plumbing"
)

// checkpointRemoteName is the git remote name used for the dedicated checkpoint remote.
const checkpointRemoteName = "entire-checkpoints"

// checkpointRemoteReachabilityTimeout is the timeout for testing remote reachability.
const checkpointRemoteReachabilityTimeout = 10 * time.Second

// checkpointRemoteFetchTimeout is the timeout for fetching branches from the remote.
const checkpointRemoteFetchTimeout = 30 * time.Second

// resolveCheckpointRemote determines the remote to use for checkpoint branch operations.
// If a checkpoint_remote URL is configured in settings:
//   - Ensures a git remote named "entire-checkpoints" is configured with that URL
//   - Tests reachability of the remote
//   - Sets up branch tracking (fetches branches that exist remotely but not locally)
//   - Returns "entire-checkpoints" as the remote name
//
// Falls back to the provided defaultRemote if:
//   - No checkpoint_remote is configured
//   - The remote is unreachable
//   - Any error occurs during setup
func resolveCheckpointRemote(ctx context.Context, defaultRemote string) string {
	s, err := settings.Load(ctx)
	if err != nil {
		return defaultRemote
	}

	remoteURL := s.GetCheckpointRemote()
	if remoteURL == "" {
		return defaultRemote
	}

	// Ensure the git remote exists with the correct URL
	if err := ensureGitRemote(ctx, checkpointRemoteName, remoteURL); err != nil {
		logging.Warn(ctx, "checkpoint-remote: failed to configure git remote",
			slog.String("url", remoteURL),
			slog.String("error", err.Error()),
		)
		return defaultRemote
	}

	// Test reachability
	if !isRemoteReachable(ctx, remoteURL) {
		logging.Info(ctx, "checkpoint-remote: unreachable, using default remote",
			slog.String("url", remoteURL),
			slog.String("default_remote", defaultRemote),
		)
		return defaultRemote
	}

	// Set up branches from the remote
	for _, branchName := range []string{paths.MetadataBranchName, paths.TrailsBranchName} {
		if err := ensureBranchFromRemote(ctx, checkpointRemoteName, branchName); err != nil {
			logging.Warn(ctx, "checkpoint-remote: failed to set up branch",
				slog.String("branch", branchName),
				slog.String("error", err.Error()),
			)
		}
	}

	return checkpointRemoteName
}

// ensureGitRemote creates or updates a git remote to point to the given URL.
func ensureGitRemote(ctx context.Context, name, url string) error {
	// Check if remote already exists and get its current URL
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", name)
	output, err := cmd.Output()
	if err != nil {
		// Remote doesn't exist, create it
		addCmd := exec.CommandContext(ctx, "git", "remote", "add", name, url)
		if addErr := addCmd.Run(); addErr != nil {
			return fmt.Errorf("failed to add remote: %w", addErr)
		}
		return nil
	}

	// Remote exists, check if URL matches
	currentURL := strings.TrimSpace(string(output))
	if currentURL == url {
		return nil
	}

	// URL differs, update it
	setCmd := exec.CommandContext(ctx, "git", "remote", "set-url", name, url)
	if setErr := setCmd.Run(); setErr != nil {
		return fmt.Errorf("failed to update remote URL: %w", setErr)
	}

	return nil
}

// isRemoteReachable tests if a git remote URL is reachable.
// Uses git ls-remote with a short timeout to avoid blocking.
func isRemoteReachable(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, checkpointRemoteReachabilityTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", url)
	cmd.Stdin = nil
	cmd.Stdout = nil // Discard output
	cmd.Stderr = nil // Discard stderr
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // Prevent interactive auth prompts
	)

	return cmd.Run() == nil
}

// ensureBranchFromRemote ensures a local branch is set up from the checkpoint remote.
// If the branch doesn't exist locally but exists on the remote, it creates the local branch.
// If the branch exists locally with a different remote tracking ref, it updates the tracking.
func ensureBranchFromRemote(ctx context.Context, remote, branchName string) error {
	repo, err := OpenRepository(ctx)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Fetch the branch from the remote
	fetchCtx, cancel := context.WithTimeout(ctx, checkpointRemoteFetchTimeout)
	defer cancel()

	refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branchName, remote, branchName)
	fetchCmd := exec.CommandContext(fetchCtx, "git", "fetch", "--no-tags", remote, refSpec)
	fetchCmd.Stdin = nil
	fetchCmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
	)
	// Fetch may fail if the branch doesn't exist on the remote yet - that's fine
	fetchErr := fetchCmd.Run()

	branchRef := plumbing.NewBranchReferenceName(branchName)
	_, localErr := repo.Reference(branchRef, true)

	if localErr != nil && fetchErr == nil {
		// Branch doesn't exist locally but fetch succeeded (exists on remote)
		// Create local branch from the remote ref
		remoteRefName := plumbing.NewRemoteReferenceName(remote, branchName)
		remoteRef, err := repo.Reference(remoteRefName, true)
		if err != nil {
			// Fetch succeeded but remote ref not found - branch may not exist on remote
			return nil
		}

		newRef := plumbing.NewHashReference(branchRef, remoteRef.Hash())
		if err := repo.Storer.SetReference(newRef); err != nil {
			return fmt.Errorf("failed to create local branch from remote: %w", err)
		}

		fmt.Fprintf(os.Stderr, "[entire] Fetched %s from %s\n", branchName, remote)
	}

	return nil
}
