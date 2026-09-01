package vault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestService_RenameTag(t *testing.T) {
	ctx := context.Background()

	t.Run("renames a frontmatter sequence entry, preserving key order", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/a.md",
			"---\ntitle: A\ntags:\n  - keep\n  - old\nauthor: me\n---\n\n# Body\n",
			vault.WriteModeOverwrite))

		renames, err := svc.RenameTag(ctx, "old", "new")
		require.NoError(t, err)
		require.Len(t, renames, 1)
		assert.Equal(t, "Notes/a.md", renames[0].Path)
		assert.True(t, renames[0].FrontmatterHit)
		assert.Equal(t, 0, renames[0].InlineCount)

		note, err := svc.ReadNote(ctx, "Notes/a.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "- new")
		assert.NotContains(t, note.Content, "- old")
		// key order preserved: title before tags before author
		titleIdx := indexOf(note.Content, "title:")
		tagsIdx := indexOf(note.Content, "tags:")
		authorIdx := indexOf(note.Content, "author:")
		assert.True(t, titleIdx < tagsIdx && tagsIdx < authorIdx, "frontmatter key order must be preserved")
	})

	t.Run("renames inline occurrences and skips fenced code blocks", func(t *testing.T) {
		svc := newLinkTestVault(t)
		content := "# Body\n\nSee #old for details.\n\n```\n#old in a code block must stay\n```\n\nAlso #old again.\n"
		require.NoError(t, svc.WriteNote(ctx, "Notes/b.md", content, vault.WriteModeOverwrite))

		renames, err := svc.RenameTag(ctx, "old", "new")
		require.NoError(t, err)
		require.Len(t, renames, 1)
		assert.False(t, renames[0].FrontmatterHit)
		assert.Equal(t, 2, renames[0].InlineCount)

		note, err := svc.ReadNote(ctx, "Notes/b.md")
		require.NoError(t, err)
		assert.Contains(t, note.Content, "#old in a code block must stay")
		assert.Contains(t, note.Content, "See #new for details.")
		assert.Contains(t, note.Content, "Also #new again.")
	})

	t.Run("notes with neither occurrence are left untouched and not reported", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/c.md", "# Body\n\nNo tags here.\n", vault.WriteModeOverwrite))

		renames, err := svc.RenameTag(ctx, "old", "new")
		require.NoError(t, err)
		assert.Empty(t, renames)
	})

	t.Run("rejects empty tag names", func(t *testing.T) {
		svc := newLinkTestVault(t)
		_, err := svc.RenameTag(ctx, "", "new")
		require.Error(t, err)
	})
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
