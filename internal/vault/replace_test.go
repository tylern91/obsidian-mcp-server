package vault_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestService_ReplaceInNote(t *testing.T) {
	ctx := context.Background()

	t.Run("literal replace, unbounded", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/a.md", "foo bar foo baz foo\n", vault.WriteModeOverwrite))

		result, err := svc.ReplaceInNote(ctx, "Notes/a.md", "foo", "qux", false, 0)
		require.NoError(t, err)
		assert.Equal(t, 3, result.OccurrencesFound)
		assert.Equal(t, 3, result.Replaced)

		note, err := svc.ReadNote(ctx, "Notes/a.md")
		require.NoError(t, err)
		assert.Equal(t, "qux bar qux baz qux\n", note.Content)
	})

	t.Run("regex replace with backreferences", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/b.md", "call foo(1) and foo(2)\n", vault.WriteModeOverwrite))

		result, err := svc.ReplaceInNote(ctx, "Notes/b.md", `foo\((\d+)\)`, "bar($1)", true, 0)
		require.NoError(t, err)
		assert.Equal(t, 2, result.OccurrencesFound)
		assert.Equal(t, 2, result.Replaced)

		note, err := svc.ReadNote(ctx, "Notes/b.md")
		require.NoError(t, err)
		assert.Equal(t, "call bar(1) and bar(2)\n", note.Content)
	})

	t.Run("maxOccurrences caps replacement but reports the true total", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/c.md", "x x x x\n", vault.WriteModeOverwrite))

		result, err := svc.ReplaceInNote(ctx, "Notes/c.md", "x", "y", false, 2)
		require.NoError(t, err)
		assert.Equal(t, 4, result.OccurrencesFound)
		assert.Equal(t, 2, result.Replaced)

		note, err := svc.ReadNote(ctx, "Notes/c.md")
		require.NoError(t, err)
		assert.Equal(t, "y y x x\n", note.Content)
	})

	t.Run("zero occurrences found leaves the note untouched", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/d.md", "nothing to see here\n", vault.WriteModeOverwrite))

		result, err := svc.ReplaceInNote(ctx, "Notes/d.md", "missing", "replacement", false, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, result.OccurrencesFound)
		assert.Equal(t, 0, result.Replaced)

		note, err := svc.ReadNote(ctx, "Notes/d.md")
		require.NoError(t, err)
		assert.Equal(t, "nothing to see here\n", note.Content)
	})

	t.Run("invalid regex pattern returns an error", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/e.md", "content\n", vault.WriteModeOverwrite))

		_, err := svc.ReplaceInNote(ctx, "Notes/e.md", "(unclosed", "x", true, 0)
		require.Error(t, err)
	})

	t.Run("result exceeding the size cap is rejected", func(t *testing.T) {
		svc := newLinkTestVault(t)
		require.NoError(t, svc.WriteNote(ctx, "Notes/f.md", "X\n", vault.WriteModeOverwrite))

		huge := strings.Repeat("a", 17*1024*1024)
		_, err := svc.ReplaceInNote(ctx, "Notes/f.md", "X", huge, false, 0)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "large") || strings.Contains(err.Error(), "size"))

		note, err := svc.ReadNote(ctx, "Notes/f.md")
		require.NoError(t, err)
		assert.Equal(t, "X\n", note.Content, "note must be untouched when the replacement would exceed the cap")
	})

	t.Run("note not found", func(t *testing.T) {
		svc := newLinkTestVault(t)
		_, err := svc.ReplaceInNote(ctx, "Notes/ghost.md", "a", "b", false, 0)
		require.Error(t, err)
	})
}
