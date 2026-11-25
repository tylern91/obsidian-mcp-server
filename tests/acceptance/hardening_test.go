package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardening_Acceptance covers the path-guard and note-mutation fixes
// shipped in v0.3.3, driven through the real tool dispatch path.
func TestHardening_Acceptance(t *testing.T) {
	t.Run("[AC-1] an ignored directory is blocked even when addressed with different letter case", func(t *testing.T) {
		c, dir := newToolsClient(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "secret.md"), []byte("hidden"), 0o644))

		for _, p := range []string{".git/secret.md", ".GIT/secret.md"} {
			read := callTool(t, c, "read_note", map[string]any{"path": p})
			assert.True(t, read.IsError, "read of %q must be blocked", p)
		}

		write := callTool(t, c, "write_note", map[string]any{"path": ".GIT/secret.md", "content": "overwritten"})
		assert.True(t, write.IsError, "write through a case-variant of an ignored dir must be blocked")
		got, err := os.ReadFile(filepath.Join(dir, ".git", "secret.md"))
		require.NoError(t, err)
		assert.Equal(t, "hidden", string(got))
	})

	t.Run("[AC-2] a symlink to an ignored target is blocked for read and for write", func(t *testing.T) {
		c, dir := newToolsClient(t)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		target := filepath.Join(dir, ".git", "secret.md")
		require.NoError(t, os.WriteFile(target, []byte("hidden"), 0o644))
		require.NoError(t, os.Symlink(target, filepath.Join(dir, "Notes", "alias.md")))

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/alias.md"})
		assert.True(t, read.IsError, "read through a symlink to an ignored path must be blocked")

		write := callTool(t, c, "write_note", map[string]any{"path": "Notes/alias.md", "content": "overwritten"})
		assert.True(t, write.IsError, "write through a symlink to an ignored path must be blocked")
		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "hidden", string(got))
	})

	t.Run("[AC-3] a symlink escaping the vault root is blocked for read and for write", func(t *testing.T) {
		c, dir := newToolsClient(t)
		outside := filepath.Join(t.TempDir(), "escape.md")
		require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "Notes", "escape.md")))

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/escape.md"})
		assert.True(t, read.IsError, "read through an escaping symlink must be blocked")

		write := callTool(t, c, "write_note", map[string]any{"path": "Notes/escape.md", "content": "overwritten"})
		assert.True(t, write.IsError, "write through an escaping symlink must be blocked")
		got, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, "outside", string(got))
	})

	t.Run("[AC-4] delete_note and move_note on a directory return NOT_A_NOTE", func(t *testing.T) {
		c, dir := newToolsClient(t)

		del := callTool(t, c, "delete_note", map[string]any{"path": "Nested", "confirm": "Nested"})
		require.True(t, del.IsError)
		assert.Contains(t, toolText(t, del), "NOT_A_NOTE")

		mv := callTool(t, c, "move_note", map[string]any{"src": "Nested", "dst": "Moved", "confirm": "Nested"})
		require.True(t, mv.IsError)
		assert.Contains(t, toolText(t, mv), "NOT_A_NOTE")

		info, err := os.Stat(filepath.Join(dir, "Nested"))
		require.NoError(t, err)
		assert.True(t, info.IsDir(), "the directory must be left in place")
	})

	t.Run("[AC-5] move_note with a partial link-rewrite failure reports success with linksComplete false", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("a read-only file does not block writes for root")
		}
		c, dir := newToolsClient(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Notes", "locked.md"), []byte("see [[simple]]\n"), 0o444))

		result := callTool(t, c, "move_note", map[string]any{
			"src":         "Notes/simple.md",
			"dst":         "Notes/renamed.md",
			"confirm":     "Notes/simple.md",
			"updateLinks": true,
		})
		require.False(t, result.IsError, "the file already moved, so the call must not report failure")

		m := toolJSON(t, result)
		assert.Equal(t, true, m["moved"])
		assert.Equal(t, false, m["linksComplete"])
		assert.NotEmpty(t, m["linkRewriteError"])

		_, err := os.Stat(filepath.Join(dir, "Notes", "renamed.md"))
		assert.NoError(t, err)
	})

	t.Run("[AC-6] a CRLF note keeps a single frontmatter block after a frontmatter update", func(t *testing.T) {
		c, dir := newToolsClient(t)
		crlf := "---\r\ntitle: Old\r\n---\r\nBody line\r\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Notes", "crlf.md"), []byte(crlf), 0o644))

		result := callTool(t, c, "update_frontmatter", map[string]any{
			"path":    "Notes/crlf.md",
			"updates": map[string]any{"status": "done"},
		})
		require.False(t, result.IsError, toolText(t, result))

		got, err := os.ReadFile(filepath.Join(dir, "Notes", "crlf.md"))
		require.NoError(t, err)
		text := string(got)
		assert.Equal(t, 2, strings.Count(text, "---"), "exactly one frontmatter block: %q", text)
		assert.Contains(t, text, "status: done")
		assert.Contains(t, text, "title: Old")
		assert.Contains(t, text, "Body line")
	})

	t.Run("[AC-7] replace_in_note rejects an empty pattern and leaves the note unchanged", func(t *testing.T) {
		c, dir := newToolsClient(t)
		before, err := os.ReadFile(filepath.Join(dir, "Notes", "simple.md"))
		require.NoError(t, err)

		result := callTool(t, c, "replace_in_note", map[string]any{
			"path": "Notes/simple.md", "pattern": "", "replacement": "X",
		})
		assert.True(t, result.IsError)

		after, err := os.ReadFile(filepath.Join(dir, "Notes", "simple.md"))
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	})

	t.Run("[AC-8] replace_in_note rejects a regex that matches the empty string", func(t *testing.T) {
		c, dir := newToolsClient(t)
		before, err := os.ReadFile(filepath.Join(dir, "Notes", "simple.md"))
		require.NoError(t, err)

		result := callTool(t, c, "replace_in_note", map[string]any{
			"path": "Notes/simple.md", "pattern": "x*", "replacement": "X", "isRegex": true,
		})
		assert.True(t, result.IsError)

		after, err := os.ReadFile(filepath.Join(dir, "Notes", "simple.md"))
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	})
}
