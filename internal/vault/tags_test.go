package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// ----------------------------------------------------------------------------
// ExtractInlineTags
// ----------------------------------------------------------------------------

func TestExtractInlineTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "no tags",
			body: "Just plain text, no hashtags here.",
			want: nil,
		},
		{
			name: "single inline tag",
			body: "Working on #obsidian today.",
			want: []string{"obsidian"},
		},
		{
			name: "multiple inline tags",
			body: "Working on #obsidian and #golang performance.",
			want: []string{"obsidian", "golang"},
		},
		{
			name: "tag at start of line",
			body: "#todo finish the implementation",
			want: []string{"todo"},
		},
		{
			name: "deduplicates case-sensitively",
			body: "#golang is great. #golang is fun. #Golang is different.",
			want: []string{"golang", "Golang"},
		},
		{
			name: "nested tag with slash",
			body: "Working on #parent/child feature.",
			want: []string{"parent/child"},
		},
		{
			name: "tag with hyphen",
			body: "Status: #in-progress",
			want: []string{"in-progress"},
		},
		{
			name: "unicode tag",
			body: "Notes sur #café et #resumé.",
			want: []string{"café", "resumé"},
		},
		{
			name: "URL hash not a tag",
			body: "See https://example.com/#anchor for more.",
			want: nil,
		},
		{
			name: "fixture: tagged.md body tags",
			body: "\nWorking on the #obsidian integration today. Also thinking about #golang performance.\n\n#todo finish the implementation\n",
			want: []string{"obsidian", "golang", "todo"},
		},
		// ----- code-fence exclusion tests -----
		{
			name: "tag inside backtick fence is not extracted",
			body: "prose here\n```\n#fake_tag inside fence\n```\nmore prose",
			want: nil,
		},
		{
			name: "tag inside tilde fence is not extracted",
			body: "prose here\n~~~\n#fake_tag inside tilde fence\n~~~\nmore prose",
			want: nil,
		},
		{
			name: "prose tag is extracted even when fence tag is present",
			body: "```\n#fenced_only\n```\nReal prose with #real_tag here.",
			want: []string{"real_tag"},
		},
		{
			name: "inline code span tag is not extracted",
			body: "Use `#not_a_tag` in your code.",
			want: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := vault.ExtractInlineTags(tc.body)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// ExtractFrontmatterTags
// ----------------------------------------------------------------------------

func TestExtractFrontmatterTags(t *testing.T) {
	t.Parallel()

	t.Run("nil tags key", func(t *testing.T) {
		t.Parallel()
		fm := map[string]any{"title": "Test"}
		assert.Empty(t, vault.ExtractFrontmatterTags(fm))
	})

	t.Run("tags as []any", func(t *testing.T) {
		t.Parallel()
		fm := map[string]any{"tags": []any{"project", "planning"}}
		assert.Equal(t, []string{"project", "planning"}, vault.ExtractFrontmatterTags(fm))
	})

	t.Run("tags as comma-separated string", func(t *testing.T) {
		t.Parallel()
		fm := map[string]any{"tags": "project, planning"}
		got := vault.ExtractFrontmatterTags(fm)
		assert.Contains(t, got, "project")
		assert.Contains(t, got, "planning")
	})

	t.Run("tags as single string scalar", func(t *testing.T) {
		t.Parallel()
		fm := map[string]any{"tags": "solo"}
		assert.Equal(t, []string{"solo"}, vault.ExtractFrontmatterTags(fm))
	})

	t.Run("empty []any", func(t *testing.T) {
		t.Parallel()
		fm := map[string]any{"tags": []any{}}
		assert.Empty(t, vault.ExtractFrontmatterTags(fm))
	})
}

// ----------------------------------------------------------------------------
// Service.ListTags
// ----------------------------------------------------------------------------

func TestService_ListTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService(testVaultRoot)

	t.Run("tagged.md returns fm and inline tags", func(t *testing.T) {
		t.Parallel()
		tags, err := svc.ListTags(ctx, "Notes/tagged.md")
		require.NoError(t, err)

		// FM tags come first.
		assert.Equal(t, "project", tags[0])
		assert.Equal(t, "planning", tags[1])

		// Inline tags follow.
		assert.Contains(t, tags, "obsidian")
		assert.Contains(t, tags, "golang")
		assert.Contains(t, tags, "todo")
	})

	t.Run("simple.md returns no tags", func(t *testing.T) {
		t.Parallel()
		tags, err := svc.ListTags(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("non-existent note returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		_, err := svc.ListTags(ctx, "Notes/missing.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}

// ----------------------------------------------------------------------------
// Service.AddTag
// ----------------------------------------------------------------------------

func makeTempNote(t *testing.T, content string) (vaultRoot, notePath string) {
	t.Helper()
	dir := t.TempDir()
	notePath = "note.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, notePath), []byte(content), 0644))
	return dir, notePath
}

func TestService_AddTag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("add tag to frontmatter creates tags key", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "---\ntitle: Test\n---\n\n# Body\n")
		svc := newService(root)

		require.NoError(t, svc.AddTag(ctx, path, "newtag", "frontmatter"))

		tags, err := svc.ListTags(ctx, path)
		require.NoError(t, err)
		assert.Contains(t, tags, "newtag")
	})

	t.Run("add tag to existing frontmatter tags", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "---\ntags:\n  - existing\n---\n\n# Body\n")
		svc := newService(root)

		require.NoError(t, svc.AddTag(ctx, path, "added", "frontmatter"))

		tags, err := svc.ListTags(ctx, path)
		require.NoError(t, err)
		assert.Contains(t, tags, "existing")
		assert.Contains(t, tags, "added")
	})

	t.Run("add duplicate frontmatter tag is no-op", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "---\ntags:\n  - existing\n---\n\n# Body\n")
		svc := newService(root)

		require.NoError(t, svc.AddTag(ctx, path, "existing", "frontmatter"))

		tags, err := svc.ListTags(ctx, path)
		require.NoError(t, err)

		count := 0
		for _, t := range tags {
			if t == "existing" {
				count++
			}
		}
		assert.Equal(t, 1, count, "tag should appear exactly once")
	})

	t.Run("add inline tag appends to body", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "# Simple Note\n\nContent here.\n")
		svc := newService(root)

		require.NoError(t, svc.AddTag(ctx, path, "inline", "inline"))

		note, err := svc.ReadNote(ctx, path)
		require.NoError(t, err)
		assert.Contains(t, note.Content, "#inline")
	})

	t.Run("invalid location returns error", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "# Note\n")
		svc := newService(root)

		err := svc.AddTag(ctx, path, "tag", "invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid location")
	})

	t.Run("non-existent file returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := newService(dir)

		err := svc.AddTag(ctx, "missing.md", "tag", "frontmatter")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}

// ----------------------------------------------------------------------------
// Service.RemoveTag
// ----------------------------------------------------------------------------

func TestService_RemoveTag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("removes tag from frontmatter sequence", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "---\ntags:\n  - keep\n  - remove\n---\n\n# Body\n")
		svc := newService(root)

		require.NoError(t, svc.RemoveTag(ctx, path, "remove"))

		tags, err := svc.ListTags(ctx, path)
		require.NoError(t, err)
		assert.Contains(t, tags, "keep")
		assert.NotContains(t, tags, "remove")
	})

	t.Run("removing absent tag is a no-op", func(t *testing.T) {
		t.Parallel()
		root, path := makeTempNote(t, "---\ntags:\n  - keep\n---\n\n# Body\n")
		svc := newService(root)

		require.NoError(t, svc.RemoveTag(ctx, path, "absent"))

		tags, err := svc.ListTags(ctx, path)
		require.NoError(t, err)
		assert.Contains(t, tags, "keep")
	})

	t.Run("non-existent file returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := newService(dir)

		err := svc.RemoveTag(ctx, "missing.md", "tag")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}

// ----------------------------------------------------------------------------
// Service.AggregateTags — code-fence exclusion
// ----------------------------------------------------------------------------

func TestAggregateTags_ExcludesFencedTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Note contains a fenced tag (#fake_tag) that must NOT be counted,
	// and a prose tag (#real_tag) that must be counted.
	content := "# Fenced Tag Test\n\n" +
		"```go\n" +
		"// #fake_tag should not be counted\n" +
		"```\n\n" +
		"Real prose uses #real_tag here.\n"

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.md"), []byte(content), 0644))

	svc := newService(dir)

	counts, err := svc.AggregateTags(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, counts["real_tag"], "real_tag should appear once")
	assert.Equal(t, 0, counts["fake_tag"], "fake_tag inside code fence should not be counted")
}
