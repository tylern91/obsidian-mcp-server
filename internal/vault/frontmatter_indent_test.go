package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indentNote writes content to a fresh vault and returns the service.
func indentNote(t *testing.T, content string) (root string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "n.md"), []byte(content), 0644))
	return root
}

func readRaw(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "n.md"))
	require.NoError(t, err)
	return string(b)
}

func TestFrontmatterIndentPreserved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const two = "---\ntitle: T\nlist:\n  - q\n  - r\naliases: [x, y]\nother: 1\n---\nbody\n"
	const four = "---\ntitle: T\nlist:\n    - q\n    - r\naliases: [x, y]\nother: 1\n---\nbody\n"

	t.Run("update_frontmatter keeps 2-space list and flow sequence", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, two)
		require.NoError(t, newService(root).UpdateFrontmatter(ctx, "n.md", map[string]any{"other": 2}, nil))
		assert.Equal(t, "---\ntitle: T\nlist:\n  - q\n  - r\naliases: [x, y]\nother: 2\n---\nbody\n", readRaw(t, root))
	})

	t.Run("update_frontmatter keeps 4-space list", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, four)
		require.NoError(t, newService(root).UpdateFrontmatter(ctx, "n.md", map[string]any{"other": 2}, nil))
		assert.Equal(t, "---\ntitle: T\nlist:\n    - q\n    - r\naliases: [x, y]\nother: 2\n---\nbody\n", readRaw(t, root))
	})

	t.Run("add tag keeps untouched 2-space list", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, "---\ntitle: T\nlist:\n  - q\ntags:\n  - a\naliases: [x, y]\n---\nbody\n")
		require.NoError(t, newService(root).AddTag(ctx, "n.md", "b", "frontmatter"))
		assert.Equal(t, "---\ntitle: T\nlist:\n  - q\ntags:\n  - a\n  - b\naliases: [x, y]\n---\nbody\n", readRaw(t, root))
	})

	t.Run("remove tag keeps untouched 4-space list", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, "---\nlist:\n    - q\ntags:\n    - a\n    - b\n---\nbody\n")
		require.NoError(t, newService(root).RemoveTag(ctx, "n.md", "b"))
		assert.Equal(t, "---\nlist:\n    - q\ntags:\n    - a\n---\nbody\n", readRaw(t, root))
	})

	t.Run("rename tag keeps untouched 2-space list", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, "---\ntitle: T\nlist:\n  - q\ntags:\n  - old\naliases: [x, y]\n---\nbody\n")
		_, err := newService(root).RenameTag(ctx, "old", "new")
		require.NoError(t, err)
		assert.Equal(t, "---\ntitle: T\nlist:\n  - q\ntags:\n  - new\naliases: [x, y]\n---\nbody\n", readRaw(t, root))
	})

	t.Run("no sequences defaults to 2-space for new lists", func(t *testing.T) {
		t.Parallel()
		root := indentNote(t, "---\ntitle: T\n---\nbody\n")
		require.NoError(t, newService(root).UpdateFrontmatter(ctx, "n.md", map[string]any{"list": []string{"a"}}, nil))
		assert.Equal(t, "---\ntitle: T\nlist:\n  - a\n---\nbody\n", readRaw(t, root))
	})
}
