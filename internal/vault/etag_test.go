package vault_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestEtag_MatchesSHA256(t *testing.T) {
	data := []byte("hello world")
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	assert.Equal(t, want, vault.Etag(data))
}

func TestService_ReadNote_EmitsEtag(t *testing.T) {
	ctx := context.Background()
	svc := newTempVault(t)

	note, err := svc.ReadNote(ctx, "Notes/simple.md")
	require.NoError(t, err)
	assert.Equal(t, vault.Etag([]byte(note.Content)), note.Etag)
	assert.NotEmpty(t, note.Etag)
}

func TestService_StatNote_EmitsEtag(t *testing.T) {
	ctx := context.Background()
	svc := newTempVault(t)

	note, err := svc.ReadNote(ctx, "Notes/simple.md")
	require.NoError(t, err)

	info, err := svc.StatNote(ctx, "Notes/simple.md")
	require.NoError(t, err)
	assert.Equal(t, note.Etag, info.Etag, "StatNote's etag must match ReadNote's etag for the same content")
}

func TestService_WriteNote_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		err = svc.WriteNote(ctx, "Notes/simple.md", "new content", vault.WriteModeOverwrite, vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
	})

	t.Run("mismatched if_match returns revision conflict", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.WriteNote(ctx, "Notes/simple.md", "new content", vault.WriteModeOverwrite, vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))
	})

	t.Run("if_match against a note that doesn't exist yet is a conflict, not an implicit create", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.WriteNote(ctx, "Notes/does-not-exist.md", "content", vault.WriteModeOverwrite, vault.WithIfMatch("any-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))
	})

	t.Run("no if_match writes unconditionally", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.WriteNote(ctx, "Notes/simple.md", "new content", vault.WriteModeOverwrite)
		require.NoError(t, err)
	})
}

func TestService_PatchNote_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict, patch not applied", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "after",
			Content:  "should not be applied",
		}, vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))

		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.NotContains(t, note.Content, "should not be applied")
	})

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		err = svc.PatchNote(ctx, "Notes/simple.md", vault.PatchOp{
			Heading:  "Simple Note",
			Position: "after",
			Content:  "applied fine",
		}, vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
	})
}

func TestService_UpdateFrontmatter_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.UpdateFrontmatter(ctx, "Notes/with-fm.md", map[string]any{"status": "done"}, nil, vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))
	})

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/with-fm.md")
		require.NoError(t, err)

		err = svc.UpdateFrontmatter(ctx, "Notes/with-fm.md", map[string]any{"status": "done"}, nil, vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
	})
}

func TestService_AddTag_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.AddTag(ctx, "Notes/simple.md", "newtag", "frontmatter", vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))
	})

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		err = svc.AddTag(ctx, "Notes/simple.md", "newtag", "frontmatter", vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
	})
}

func TestService_RemoveTag_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.RemoveTag(ctx, "Notes/simple.md", "sometag", vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))
	})
}

func TestService_DeleteNote_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict, note not deleted", func(t *testing.T) {
		svc := newTempVault(t)
		err := svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false, vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err, "note must still exist after a conflicting delete")
	})

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		err = svc.DeleteNote(ctx, "Notes/simple.md", "Notes/simple.md", false, vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
	})
}

func TestService_MoveNote_IfMatch(t *testing.T) {
	ctx := context.Background()

	t.Run("mismatched if_match returns revision conflict, note not moved", func(t *testing.T) {
		svc := newTempVault(t)
		_, err := svc.MoveNote(ctx, "Notes/simple.md", "Archive/simple.md", "Notes/simple.md", false, false, vault.WithIfMatch("stale-etag"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrRevisionConflict))

		_, err = svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err, "src must still exist after a conflicting move")
	})

	t.Run("matching if_match succeeds", func(t *testing.T) {
		svc := newTempVault(t)
		note, err := svc.ReadNote(ctx, "Notes/simple.md")
		require.NoError(t, err)

		result, err := svc.MoveNote(ctx, "Notes/simple.md", "Archive/simple.md", "Notes/simple.md", false, false, vault.WithIfMatch(note.Etag))
		require.NoError(t, err)
		assert.True(t, result.Moved)
	})

	t.Run("dryRun does not enforce if_match", func(t *testing.T) {
		svc := newTempVault(t)
		result, err := svc.MoveNote(ctx, "Notes/simple.md", "Archive/simple.md", "Notes/simple.md", false, true, vault.WithIfMatch("stale-etag"))
		require.NoError(t, err, "dry run must not enforce if_match")
		assert.False(t, result.Moved)
	})
}
