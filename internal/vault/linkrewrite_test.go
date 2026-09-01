package vault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func newLinkTestVault(t *testing.T) *vault.Service {
	t.Helper()
	return vault.New(t.TempDir(), nil)
}

func TestService_RewriteLinksOnMove(t *testing.T) {
	ctx := context.Background()

	t.Run("rewrites a bare wikilink to the new basename", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "See [[simple]] for details.\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", false)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.Equal(t, "Linker.md", rewrites[0].Path)
		assert.Equal(t, "[[simple]]", rewrites[0].OldText)
		assert.Equal(t, "[[renamed]]", rewrites[0].NewText)
		assert.False(t, rewrites[0].Ambiguous)

		note, err := svc.ReadNote(ctx, "Linker.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "[[renamed]]")
	})

	t.Run("rewrites a path-style markdown link, preserving fragment", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "[link](Notes/simple.md#Intro)\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", false)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.Equal(t, "[link](Archive/renamed.md#Intro)", rewrites[0].NewText)

		note, err := svc.ReadNote(ctx, "Linker.md")
		require.NoError(t, err)
		assert.Equal(t, "[link](Archive/renamed.md#Intro)\n", note.Content)
	})

	t.Run("preserves anchor, block ref, and alias on wikilinks", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "[[simple#^abc123|display text]]\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", false)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.Equal(t, "[[renamed#^abc123|display text]]", rewrites[0].NewText)
	})

	t.Run("ambiguous bare basename is reported and left untouched", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple A\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Other/simple.md", "# Simple B\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "See [[simple]].\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", false)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.True(t, rewrites[0].Ambiguous)
		assert.Empty(t, rewrites[0].NewText)

		note, err := svc.ReadNote(ctx, "Linker.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "[[simple]]", "ambiguous link must be left untouched")
	})

	t.Run("dry run reports without writing", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "See [[simple]].\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", true)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.Equal(t, "[[renamed]]", rewrites[0].NewText)

		note, err := svc.ReadNote(ctx, "Linker.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "[[simple]]", "dry run must not write")
	})

	t.Run("URL-encoded markdown path is decoded, matched, and re-encoded", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "My Notes/simple note.md", "# Simple\n", vault.WriteModeOverwrite))
		require.NoError(t, svc.WriteNote(ctx, "Linker.md", "[link](My%20Notes/simple%20note.md)\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "My Notes/simple note.md", "Archive/simple note.md", false)
		require.NoError(t, err)
		require.Len(t, rewrites, 1)
		assert.Equal(t, "[link](Archive/simple%20note.md)", rewrites[0].NewText)
	})

	t.Run("no referrers yields an empty, non-nil-error result", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/simple.md", "# Simple\n", vault.WriteModeOverwrite))

		rewrites, err := svc.RewriteLinksOnMove(ctx, "Notes/simple.md", "Archive/renamed.md", false)
		require.NoError(t, err)
		assert.Empty(t, rewrites)
	})
}
