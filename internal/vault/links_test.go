package vault_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// ----------------------------------------------------------------------------
// ExtractLinks
// ----------------------------------------------------------------------------

func TestExtractLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "no links",
			content: "# Plain Note\n\nNo links here.",
			want:    nil,
		},
		{
			name:    "bare wikilink",
			content: "See [[simple]] for more.",
			want:    []string{"simple"},
		},
		{
			name:    "wikilink with alias strips alias",
			content: "See [[with-fm|FM Note]] for details.",
			want:    []string{"with-fm"},
		},
		{
			name:    "wikilink with anchor strips anchor",
			content: "See [[note#section]] for more.",
			want:    []string{"note"},
		},
		{
			name:    "embed wikilink",
			content: "Embedded: ![[simple]]",
			want:    []string{"simple"},
		},
		{
			name:    "markdown link ending in .md",
			content: "A [link](Nested/Deep/note.md) to a note.",
			want:    []string{"Nested/Deep/note.md"},
		},
		{
			name:    "markdown link not ending in .md ignored",
			content: "A [link](https://example.com) to a site.",
			want:    nil,
		},
		{
			name: "linked.md: links returned in document order",
			// Wikilink, alias-wikilink, markdown-link, embed, wikilink-with-path.
			content: "A wikilink to [[simple]] and another to [[with-fm|FM Note]].\n" +
				"A markdown link to [the nested note](Nested/Deep/note.md).\n" +
				"An embedded note: ![[simple]]\n" +
				"Reference to [[Notes/unicode]] for unicode content.\n",
			want: []string{"simple", "with-fm", "Nested/Deep/note.md", "Notes/unicode"},
		},
		{
			name:    "deduplicates wikilink and embed to same target",
			content: "[[simple]] and ![[simple]]",
			want:    []string{"simple"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := vault.ExtractLinks(tc.content)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Service.GetBacklinks
// ----------------------------------------------------------------------------

func TestService_GetBacklinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService(testVaultRoot)

	t.Run("simple.md backlinks include linked.md at expected lines", func(t *testing.T) {
		t.Parallel()
		backlinks, err := svc.GetBacklinks(ctx, "Notes/simple.md")
		require.NoError(t, err)

		// There may be multiple notes referencing simple.md — verify linked.md is present.
		// linked.md has two occurrences at lines 9 (wikilink) and 13 (embed).
		linkedBacklinks := make([]vault.Backlink, 0)
		for _, bl := range backlinks {
			if bl.Path == "Notes/linked.md" {
				linkedBacklinks = append(linkedBacklinks, bl)
			}
		}

		assert.Len(t, linkedBacklinks, 2, "linked.md should have 2 backlinks to simple.md")

		lines := []int{linkedBacklinks[0].Line, linkedBacklinks[1].Line}
		assert.Contains(t, lines, 9, "wikilink [[simple]] on line 9")
		assert.Contains(t, lines, 13, "embed ![[simple]] on line 13")

		for _, bl := range linkedBacklinks {
			assert.NotEmpty(t, bl.Snippet)
		}
	})

	t.Run("with-fm.md has backlink from linked.md line 9", func(t *testing.T) {
		t.Parallel()
		backlinks, err := svc.GetBacklinks(ctx, "Notes/with-fm.md")
		require.NoError(t, err)

		found := false
		for _, bl := range backlinks {
			if bl.Path == "Notes/linked.md" && bl.Line == 9 {
				found = true
				assert.Contains(t, bl.Snippet, "with-fm")
			}
		}
		assert.True(t, found, "linked.md line 9 should reference with-fm")
	})

	t.Run("target is excluded from its own backlinks", func(t *testing.T) {
		t.Parallel()
		backlinks, err := svc.GetBacklinks(ctx, "Notes/linked.md")
		require.NoError(t, err)

		for _, bl := range backlinks {
			assert.NotEqual(t, "Notes/linked.md", bl.Path, "note must not link to itself")
		}
	})

	t.Run("non-existent target returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetBacklinks(ctx, "Notes/missing.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetBacklinks(ctx, "../outside.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrPathTraversal))
	})
}
