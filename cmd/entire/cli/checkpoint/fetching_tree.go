package checkpoint

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"

	"github.com/entireio/cli/cmd/entire/cli/logging"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
)

// BlobFetchFunc fetches missing blob objects by hash from a remote.
type BlobFetchFunc func(ctx context.Context, hashes []plumbing.Hash) error

// FetchingTree wraps a git tree to automatically fetch missing blobs on demand.
// After a treeless fetch (--filter=blob:none), tree objects are available locally
// but blob objects are not. Each File() call checks whether the target blob
// exists locally and fetches it from the remote if missing, using FindEntry
// to locate the blob hash without resolving the blob itself.
//
// Because go-git's ObjectStorage caches the packfile index and never refreshes
// it, blobs fetched by external git commands (e.g. git fetch-pack) may not be
// visible to go-git's storer. As a fallback, File() reads the blob via
// "git cat-file" which always sees the current on-disk object store.
type FetchingTree struct {
	inner    *object.Tree
	ctx      context.Context
	resolver *BlobResolver
	fetch    BlobFetchFunc
}

// NewFetchingTree wraps a git tree with on-demand blob fetching.
// The storer is used to check if blobs exist locally, and fetch is called
// to download any that are missing. If fetch is nil, File() behaves
// identically to the underlying tree.
func NewFetchingTree(ctx context.Context, tree *object.Tree, s storer.EncodedObjectStorer, fetch BlobFetchFunc) *FetchingTree {
	return &FetchingTree{
		inner:    tree,
		ctx:      ctx,
		resolver: NewBlobResolver(s),
		fetch:    fetch,
	}
}

// File returns the file at the given path. If the blob is not available
// locally (e.g. after a treeless fetch), it is fetched on demand. If go-git's
// storer still can't see the blob after fetching (due to cached packfile index),
// the blob is read via "git cat-file" and an in-memory File is returned.
func (t *FetchingTree) File(path string) (*object.File, error) {
	// Fast path: blob already available in go-git's storer.
	file, err := t.inner.File(path)
	if err == nil {
		return file, nil
	}

	if t.fetch == nil {
		return nil, err //nolint:wrapcheck // pass-through wrapper
	}

	// Find the tree entry to get the blob hash without resolving the blob.
	// FindEntry only navigates tree objects (available after --filter=blob:none).
	entry, findErr := t.inner.FindEntry(path)
	if findErr != nil {
		logging.Debug(t.ctx, "FetchingTree.File: entry not found",
			slog.String("path", path),
			slog.String("error", findErr.Error()),
		)
		return nil, err //nolint:wrapcheck // return original File() error
	}

	logging.Debug(t.ctx, "FetchingTree.File: blob missing, fetching",
		slog.String("path", path),
		slog.String("hash", entry.Hash.String()[:12]),
	)

	// Fetch the blob from the remote.
	if fetchErr := t.fetch(t.ctx, []plumbing.Hash{entry.Hash}); fetchErr != nil {
		logging.Warn(t.ctx, "FetchingTree.File: blob fetch failed",
			slog.String("path", path),
			slog.String("hash", entry.Hash.String()[:12]),
			slog.String("error", fetchErr.Error()),
		)
		return nil, err //nolint:wrapcheck // return original File() error
	}

	// Try go-git again — works if blob was stored as a loose object.
	file, err = t.inner.File(path)
	if err == nil {
		return file, nil
	}

	// go-git's storer caches the packfile index and won't see new packs
	// created by external git commands. Fall back to "git cat-file" which
	// reads directly from the on-disk object store.
	logging.Debug(t.ctx, "FetchingTree.File: storer cache stale, reading via git cat-file",
		slog.String("path", path),
		slog.String("hash", entry.Hash.String()[:12]),
	)
	return t.readFileViaGit(path, entry)
}

// readFileViaGit reads a blob via "git cat-file -p <hash>" and returns an
// in-memory *object.File. This bypasses go-git's storer which may have a
// stale packfile index after external git commands fetched new objects.
func (t *FetchingTree) readFileViaGit(path string, entry *object.TreeEntry) (*object.File, error) {
	cmd := exec.CommandContext(t.ctx, "git", "cat-file", "-p", entry.Hash.String())
	content, cmdErr := cmd.Output()
	if cmdErr != nil {
		logging.Warn(t.ctx, "FetchingTree.readFileViaGit: cat-file failed",
			slog.String("path", path),
			slog.String("hash", entry.Hash.String()[:12]),
			slog.String("error", cmdErr.Error()),
		)
		return nil, fmt.Errorf("blob %s not readable after fetch: %w", entry.Hash.String()[:12], cmdErr)
	}

	// Create an in-memory encoded object to construct the File.
	memObj := &plumbing.MemoryObject{}
	memObj.SetType(plumbing.BlobObject)
	memObj.SetSize(int64(len(content)))
	w, wErr := memObj.Writer()
	if wErr != nil {
		return nil, fmt.Errorf("memory object writer: %w", wErr)
	}
	if _, wErr = w.Write(content); wErr != nil {
		return nil, fmt.Errorf("memory object write: %w", wErr)
	}
	if wErr = w.Close(); wErr != nil {
		return nil, fmt.Errorf("memory object close: %w", wErr)
	}

	blob := &object.Blob{}
	if dErr := blob.Decode(memObj); dErr != nil {
		return nil, fmt.Errorf("blob decode: %w", dErr)
	}

	logging.Debug(t.ctx, "FetchingTree.readFileViaGit: blob read successfully",
		slog.String("path", path),
		slog.String("hash", entry.Hash.String()[:12]),
		slog.Int64("size", int64(len(content))),
	)

	return object.NewFile(path, entry.Mode, blob), nil
}

// Tree returns the subtree at the given path, wrapped with the same fetching
// behavior.
func (t *FetchingTree) Tree(path string) (*FetchingTree, error) {
	subtree, err := t.inner.Tree(path)
	if err != nil {
		return nil, fmt.Errorf("tree %s: %w", path, err)
	}
	return &FetchingTree{
		inner:    subtree,
		ctx:      t.ctx,
		resolver: t.resolver,
		fetch:    t.fetch,
	}, nil
}

// RawEntries returns the direct tree entries (no blob reads needed).
func (t *FetchingTree) RawEntries() []object.TreeEntry {
	return t.inner.Entries
}

// Unwrap returns the underlying *object.Tree.
func (t *FetchingTree) Unwrap() *object.Tree {
	return t.inner
}

// Files returns a recursive file iterator from the underlying tree.
// Warning: after a treeless fetch, this iterator will fail when it tries
// to resolve blob objects. Use File() for on-demand blob fetching instead.
func (t *FetchingTree) Files() *object.FileIter {
	return t.inner.Files()
}

// FileReader provides read access to files within a git tree.
// Both *object.Tree and *FetchingTree implement this interface.
type FileReader interface {
	File(path string) (*object.File, error)
}

// FileOpener provides access to a file's content reader.
// *object.File implements this interface.
type FileOpener interface {
	Reader() (io.ReadCloser, error)
}
