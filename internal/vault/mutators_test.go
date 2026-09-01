package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newTempVault copies the testdata vault to a temp dir so mutating tests
// don't pollute the fixture directory.
func newTempVault(t *testing.T) *vault.Service {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "vault")
	dst := t.TempDir()
	require.NoError(t, copyDir(src, dst))
	return vault.New(dst, nil)
}

// copyDir recursively copies src to dst (shallow symlinks not followed).
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// --- PatchNote ---

func TestService_PatchNote(t *testing.T) {
	ctx := context.Background()

	t.Run("insert before heading", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "before",
			Content:  "<!-- injected before heading -->",
		})
		require.NoError(t, err)

		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.True(t, strings.Contains(note.Content, "<!-- injected before heading -->\n# Simple Note"),
			"inserted text should appear before heading")
	})

	t.Run("insert after heading body", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "after",
			Content:  "## Appended Section\n\nNew content.",
		})
		require.NoError(t, err)

		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.True(t, strings.Contains(note.Content, "## Appended Section"), "appended section should appear")
	})

	t.Run("replace heading body", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "replace_body",
			Content:  "Replaced body content.",
		})
		require.NoError(t, err)

		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.True(t, strings.Contains(note.Content, "Replaced body content."), "new body should appear")
		assert.False(t, strings.Contains(note.Content, "simple note with no frontmatter"), "old body should be gone")
	})

	t.Run("heading not found", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Nonexistent Heading",
			Position: "after",
			Content:  "x",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrHeadingNotFound))
	})

	t.Run("unknown position", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "sideways",
			Content:  "x",
		})
		require.Error(t, err)
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "../escape.md", vault.PatchOp{
			Heading:  "H",
			Position: "after",
			Content:  "x",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
	})

	t.Run("symlink escape rejected", func(t *testing.T) {
		svc := newTempVault(t)
		root := svc.Root()
		// Create a note and a symlink pointing outside the vault.
		require.NoError(t, os.WriteFile(filepath.Join(root, "Notes", "real.md"), []byte("# Head\nbody"), 0644))
		outsideFile := filepath.Join(t.TempDir(), "outside.md")
		require.NoError(t, os.WriteFile(outsideFile, []byte("# Head\nbody"), 0644))
		symlinkPath := filepath.Join(root, "Notes", "link.md")
		require.NoError(t, os.Symlink(outsideFile, symlinkPath))

		err := svc.PatchNote(ctx, "Notes/link.md", vault.PatchOp{
			Heading:  "Head",
			Position: "after",
			Content:  "x",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrSymlinkEscape))
	})
}

// --- DeleteNote ---

func TestService_DeleteNote(t *testing.T) {
	ctx := context.Background()

	t.Run("default moves to trash, not permanent delete", func(t *testing.T) {
		svc := newTempVault(t)
		before, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		err = svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false)
		require.NoError(t, err)

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		assert.True(t, errors.Is(err, vault.ErrNotFound), "note should no longer be readable at its original path")

		trashed := findTrashedFile(t, svc, "Notes/simple.md")
		require.NotEmpty(t, trashed, "expected a trash entry for the deleted note")
		content, err := os.ReadFile(trashed)
		require.NoError(t, err)
		assert.Equal(t, before.Content, string(content), "trashed content must match the original")
	})

	t.Run("nested path preserves structure under the trash entry", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false)
		require.NoError(t, err)

		trashed := findTrashedFile(t, svc, "Notes/simple.md")
		require.NotEmpty(t, trashed)
		assert.True(t, strings.HasSuffix(filepath.ToSlash(trashed), "Notes/simple.md"), "trash entry must preserve the vault-relative directory structure, got %s", trashed)
	})

	t.Run("permanent hard-deletes without a trash entry", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", true)
		require.NoError(t, err)

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		assert.True(t, errors.Is(err, vault.ErrNotFound))
		assert.Empty(t, findTrashedFile(t, svc, "Notes/simple.md"), "permanent delete must not leave a trash entry")
	})

	t.Run("confirm mismatch", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/other.md", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrConfirmMismatch))
	})

	t.Run("confirm mismatch applies to permanent delete too", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/other.md", true)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrConfirmMismatch))
	})

	t.Run("not found", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/ghost.md", "Notes/ghost.md", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "../outside.md", "../outside.md", false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
	})

	t.Run("cannot target the trash directory itself", func(t *testing.T) {
		svc := newTempVault(t)
		path := ".obsidian-mcp/trash/whatever/Notes/simple.md"
		err := svc.DeleteNote(ctx, path, path, false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathRestricted))
	})
}

// findTrashedFile searches .obsidian-mcp/trash for a file whose path (under
// its timestamp directory) ends with wantRelPath, returning its absolute
// path or "" if none is found.
func findTrashedFile(t *testing.T, svc *vault.Service, wantRelPath string) string {
	t.Helper()
	trashRoot := filepath.Join(svc.Root(), ".obsidian-mcp", "trash")
	entries, err := os.ReadDir(trashRoot)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)

	want := filepath.ToSlash(wantRelPath)
	var found string
	for _, e := range entries {
		_ = filepath.Walk(filepath.Join(trashRoot, e.Name()), func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(filepath.Join(trashRoot, e.Name()), p)
			if relErr == nil && filepath.ToSlash(rel) == want {
				found = p
			}
			return nil
		})
	}
	return found
}

// --- PruneTrash ---

// trashTimestampLayout mirrors the unexported layout in mutators.go — a
// trash entry directory name is exactly this format.
const trashTimestampLayout = "20060102T150405.000000000"

// makeTrashEntry creates an empty trash entry directory timestamped at ts.
func makeTrashEntry(t *testing.T, svc *vault.Service, ts time.Time) string {
	t.Helper()
	name := ts.UTC().Format(trashTimestampLayout)
	dir := filepath.Join(svc.Root(), ".obsidian-mcp", "trash", name)
	require.NoError(t, os.MkdirAll(dir, 0755))
	return dir
}

func TestService_PruneTrash(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	t.Run("removes entries older than retention, keeps newer ones", func(t *testing.T) {
		svc := newTempVault(t)
		old := makeTrashEntry(t, svc, now.Add(-31*24*time.Hour))
		recent := makeTrashEntry(t, svc, now.Add(-1*time.Hour))

		removed, err := svc.PruneTrash(now, 30)
		require.NoError(t, err)
		assert.Equal(t, 1, removed)

		_, err = os.Stat(old)
		assert.True(t, os.IsNotExist(err), "entry older than retention should be removed")
		_, err = os.Stat(recent)
		assert.NoError(t, err, "entry within retention should survive")
	})

	t.Run("leaves non-timestamp entries alone", func(t *testing.T) {
		svc := newTempVault(t)
		stray := filepath.Join(svc.Root(), ".obsidian-mcp", "trash", "not-a-timestamp")
		require.NoError(t, os.MkdirAll(stray, 0755))

		removed, err := svc.PruneTrash(now, 30)
		require.NoError(t, err)
		assert.Equal(t, 0, removed)

		_, err = os.Stat(stray)
		assert.NoError(t, err)
	})

	t.Run("no trash directory yet is not an error", func(t *testing.T) {
		svc := newTempVault(t)
		removed, err := svc.PruneTrash(now, 30)
		require.NoError(t, err)
		assert.Equal(t, 0, removed)
	})
}

// --- MoveNote ---

func TestService_MoveNote(t *testing.T) {
	ctx := context.Background()

	t.Run("move to new path", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "Notes/simple.md", "Archive/simple.md", "Notes/simple.md")
		require.NoError(t, err)

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		assert.True(t, errors.Is(err, vault.ErrNotFound), "src should be gone")

		note, err := svc.ReadNote(ctx, "Archive/simple.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "Simple Note")
	})

	t.Run("confirm mismatch", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "Notes/simple.md", "Archive/simple.md", "wrong.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrConfirmMismatch))
	})

	t.Run("dst already exists", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "Notes/simple.md", "Notes/with-fm.md", "Notes/simple.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrAlreadyExists))
	})

	t.Run("src not found", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "Notes/ghost.md", "Archive/ghost.md", "Notes/ghost.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("src path traversal rejected", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "../escape.md", "Notes/escape.md", "../escape.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
	})

	t.Run("dst path traversal rejected", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.MoveNote(ctx, "Notes/simple.md", "../escape.md", "Notes/simple.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
	})

	// Concurrent moves to the same dst must not both succeed: the dst-exists
	// check must run inside s.mu, or two racing os.Rename calls could both
	// pass a stale check and the second silently clobbers the first.
	t.Run("concurrent moves to same dst do not both succeed", func(t *testing.T) {
		svc := newTempVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/second-src.md", "second", vault.WriteModeOverwrite))

		var wg sync.WaitGroup
		errs := make([]error, 2)
		srcs := []string{"Notes/simple.md", "Notes/second-src.md"}
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func(i int) {
				defer wg.Done()
				errs[i] = svc.MoveNote(ctx, srcs[i], "Archive/winner.md", srcs[i])
			}(i)
		}
		wg.Wait()

		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
			} else {
				assert.True(t, errors.Is(err, vault.ErrAlreadyExists), "unexpected error: %v", err)
			}
		}
		assert.Equal(t, 1, successes, "exactly one concurrent move to the same dst should succeed")
	})
}
