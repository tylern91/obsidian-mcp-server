package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func writeNoteFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func readNoteFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	return string(b)
}

// TestHeadings_Acceptance covers heading disambiguation, code-fence-aware
// heading parsing, and the rename_heading tool, driven through the real tool
// dispatch path.
func TestHeadings_Acceptance(t *testing.T) {
	const dupNote = "# A\n## Notes\nnested\n# Notes\ntop\n"

	t.Run("[AC-1] patch_note on a heading text that matches several headings fails with AMBIGUOUS_HEADING and changes nothing", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "dup.md", dupNote)

		result := callTool(t, c, "patch_note", map[string]any{
			"path": "dup.md", "heading": "Notes", "position": "replace_body", "content": "X",
		})
		require.True(t, result.IsError)
		text := toolText(t, result)
		assert.Contains(t, text, "AMBIGUOUS_HEADING")
		assert.Contains(t, text, "line 2 (level 2)")
		assert.Contains(t, text, "line 4 (level 1)")
		assert.Equal(t, dupNote, readNoteFile(t, dir, "dup.md"))
	})

	t.Run("[AC-2] patch_note with line patches exactly the heading on that line", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "dup.md", dupNote)

		result := callTool(t, c, "patch_note", map[string]any{
			"path": "dup.md", "heading": "Notes", "position": "replace_body", "content": "X", "line": 4,
		})
		require.False(t, result.IsError, toolText(t, result))
		assert.Equal(t, "# A\n## Notes\nnested\n# Notes\nX", readNoteFile(t, dir, "dup.md"))

		bad := callTool(t, c, "patch_note", map[string]any{
			"path": "dup.md", "heading": "Notes", "position": "replace_body", "content": "Y", "line": 1,
		})
		assert.True(t, bad.IsError, "a line that is not a matching heading must be rejected")
	})

	t.Run("[AC-3] a hash line inside a fenced code block is neither a patch anchor nor the end of a section", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "fenced.md", "# A\nintro\n```\n# B\n```\ntail\n# C\nc\n")

		hidden := callTool(t, c, "patch_note", map[string]any{
			"path": "fenced.md", "heading": "B", "position": "after", "content": "X",
		})
		assert.True(t, hidden.IsError, "a heading that exists only inside a fence must not be found")

		result := callTool(t, c, "patch_note", map[string]any{
			"path": "fenced.md", "heading": "A", "position": "replace_body", "content": "X",
		})
		require.False(t, result.IsError, toolText(t, result))
		assert.Equal(t, "# A\nX\n# C\nc\n", readNoteFile(t, dir, "fenced.md"))
	})

	t.Run("[AC-4] get_note_outline omits headings inside fenced code blocks", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "fenced.md", "# A\n```bash\n# comment\n```\n## B\n~~~\n### C\n~~~\n")

		result := callTool(t, c, "get_note_outline", map[string]any{"path": "fenced.md"})
		require.False(t, result.IsError, toolText(t, result))
		headings := toolJSON(t, result)["headings"].([]any)
		require.Len(t, headings, 2)
		assert.Equal(t, "A", headings[0].(map[string]any)["text"])
		assert.Equal(t, "B", headings[1].(map[string]any)["text"])
		assert.EqualValues(t, 5, headings[1].(map[string]any)["line"])
	})

	t.Run("[AC-5] rename_heading keeps the level and rewrites anchors that point at the old heading", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "Target.md", "# Top\n## Old Name\nbody\n")
		writeNoteFile(t, dir, "ref.md", "[[Target#Old Name]] [[Target#Old Name|alias]] [t](Target.md#old-name) [[Target#Other]]\n")

		result := callTool(t, c, "rename_heading", map[string]any{
			"path": "Target.md", "heading": "Old Name", "newHeading": "New Name",
		})
		require.False(t, result.IsError, toolText(t, result))
		assert.Equal(t, "# Top\n## New Name\nbody\n", readNoteFile(t, dir, "Target.md"))
		assert.Equal(t, "[[Target#New Name]] [[Target#New Name|alias]] [t](Target.md#new-name) [[Target#Other]]\n", readNoteFile(t, dir, "ref.md"))

		off := callTool(t, c, "rename_heading", map[string]any{
			"path": "Target.md", "heading": "New Name", "newHeading": "Newer", "updateLinks": false,
		})
		require.False(t, off.IsError, toolText(t, off))
		assert.Equal(t, "[[Target#New Name]] [[Target#New Name|alias]] [t](Target.md#new-name) [[Target#Other]]\n", readNoteFile(t, dir, "ref.md"), "updateLinks=false must leave links alone")
	})

	t.Run("[AC-6] rename_heading on duplicate headings fails with AMBIGUOUS_HEADING unless line selects one", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "dup.md", dupNote)

		result := callTool(t, c, "rename_heading", map[string]any{
			"path": "dup.md", "heading": "Notes", "newHeading": "Renamed",
		})
		require.True(t, result.IsError)
		assert.Contains(t, toolText(t, result), "AMBIGUOUS_HEADING")
		assert.Equal(t, dupNote, readNoteFile(t, dir, "dup.md"))

		ok := callTool(t, c, "rename_heading", map[string]any{
			"path": "dup.md", "heading": "Notes", "newHeading": "Renamed", "line": 2,
		})
		require.False(t, ok.IsError, toolText(t, ok))
		assert.Equal(t, "# A\n## Renamed\nnested\n# Notes\ntop\n", readNoteFile(t, dir, "dup.md"))
	})

	t.Run("[AC-7] rename_heading honours if_match", func(t *testing.T) {
		c, dir := newToolsClient(t)
		writeNoteFile(t, dir, "n.md", "# Old\n")

		result := callTool(t, c, "rename_heading", map[string]any{
			"path": "n.md", "heading": "Old", "newHeading": "New", "if_match": "stale",
		})
		require.True(t, result.IsError)
		assert.Contains(t, toolText(t, result), "REVISION_CONFLICT")
		assert.Equal(t, "# Old\n", readNoteFile(t, dir, "n.md"))

		etag := vault.Etag([]byte("# Old\n"))
		ok := callTool(t, c, "rename_heading", map[string]any{
			"path": "n.md", "heading": "Old", "newHeading": "New", "if_match": etag,
		})
		require.False(t, ok.IsError, toolText(t, ok))
	})

	t.Run("[AC-8] rename_heading is not offered in read-only mode", func(t *testing.T) {
		c, _ := newToolsClient(t, func(d *tools.Deps) { d.ReadOnly = true })
		listed, err := c.ListTools(t.Context(), mcp.ListToolsRequest{})
		require.NoError(t, err)
		var names []string
		for _, tl := range listed.Tools {
			names = append(names, tl.Name)
		}
		assert.NotContains(t, names, "rename_heading")
		assert.NotContains(t, names, "patch_note")
		assert.Contains(t, names, "get_note_outline", "read tools stay registered")
	})
}
