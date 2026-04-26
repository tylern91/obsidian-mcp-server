package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	t.Run("delete existing note", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md")
		require.NoError(t, err)

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("confirm mismatch", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/other.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrConfirmMismatch))
	})

	t.Run("not found", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/ghost.md", "Notes/ghost.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "../outside.md", "../outside.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
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
}
