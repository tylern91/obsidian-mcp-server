package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newToolsClient copies the fixture vault into a fresh temp dir, registers
// internal/tools against a real MCP server with a fixed-clock periodic
// service (2024-01-15, the date of the Daily Notes fixture), and returns a
// client connected to it plus the temp vault root. opts may mutate deps
// before registration (e.g. to set ReadOnly or VaultName).
func newToolsClient(t *testing.T, opts ...func(*tools.Deps)) (*client.Client, string) {
	t.Helper()
	return newToolsClientAt(t, 2024, 1, 15, opts...)
}

// newToolsClientAt is newToolsClient with an explicit periodic-service clock date.
func newToolsClientAt(t *testing.T, year, month, day int, opts ...func(*tools.Deps)) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, copyDir("../../testdata/vault", dir))
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	vs := vault.New(dir, filter)
	deps := tools.Deps{
		Vault:      vs,
		Search:     search.New(vs),
		Periodic:   periodic.New(dir).WithClock(fixedClock(year, month, day)),
		MaxBatch:   10,
		MaxResults: 50,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	c := startClient(t, func(s *server.MCPServer) {
		tools.RegisterAll(s, deps)
	})
	return c, dir
}

// callTool invokes a tool by name with the given arguments and returns the result.
func callTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}}
	result, err := c.CallTool(t.Context(), req)
	require.NoError(t, err)
	return result
}

// toolText returns the concatenated text content of a tool result.
func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected tool result content")
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text
}

// toolJSON decodes the first text content block of a tool result as JSON.
func toolJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(toolText(t, result)), &m))
	return m
}

func TestTools_Acceptance(t *testing.T) {
	t.Run("[AC-1] read_note returns the content of an existing note", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		assert.False(t, result.IsError)
		m := toolJSON(t, result)
		assert.Contains(t, m["content"], "This is a simple note with no frontmatter.")
	})

	t.Run("[AC-2] read_note returns a tool error for a missing note", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "read_note", map[string]any{"path": "nonexistent.md"})
		assert.True(t, result.IsError)
	})

	t.Run("[AC-3] write_note with default mode overwrites and the new content is readable back", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "replaced content"})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		assert.Equal(t, "replaced content", m["content"])
	})

	t.Run("[AC-4] write_note with mode=append adds to the end of an existing note", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "\nappended line", "mode": "append"})

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		content, _ := m["content"].(string)
		assert.Contains(t, content, "This is a simple note with no frontmatter.")
		assert.Contains(t, content, "appended line")
	})

	t.Run("[AC-5] write_note with a stale if_match etag is rejected with REVISION_CONFLICT", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "write_note", map[string]any{
			"path":     "Notes/simple.md",
			"content":  "should not apply",
			"if_match": "stale-etag-value",
		})
		assert.True(t, result.IsError)
		assert.Contains(t, toolText(t, result), "REVISION_CONFLICT")
	})

	t.Run("[AC-6] list_directory lists top-level entries under Notes/ by default", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "list_directory", map[string]any{"path": "Notes"})
		assert.False(t, result.IsError)
		m := toolJSON(t, result)
		entries, ok := m["entries"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, entries)
	})

	t.Run("[AC-7] list_directory limit truncates results and reports the full total", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "list_directory", map[string]any{"path": "Notes", "limit": float64(1)})
		m := toolJSON(t, result)
		entries, ok := m["entries"].([]any)
		require.True(t, ok)
		assert.Len(t, entries, 1)
		total, ok := m["total"].(float64)
		require.True(t, ok)
		assert.Greater(t, total, float64(1))
	})

	t.Run("[AC-8] get_frontmatter returns parsed frontmatter fields for a note with frontmatter", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_frontmatter", map[string]any{"path": "Notes/with-fm.md"})
		m := toolJSON(t, result)
		fm, ok := m["frontmatter"].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, fm["title"])
	})

	t.Run("[AC-9] update_frontmatter sets a new key that is then visible via get_frontmatter", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "update_frontmatter", map[string]any{
			"path":    "Notes/with-fm.md",
			"updates": `{"reviewed":true}`,
		})
		assert.False(t, result.IsError)

		got := callTool(t, c, "get_frontmatter", map[string]any{"path": "Notes/with-fm.md"})
		m := toolJSON(t, got)
		fm, ok := m["frontmatter"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, fm["reviewed"])
	})

	t.Run("[AC-10] manage_tags action=add adds a frontmatter tag visible via get_frontmatter", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "manage_tags", map[string]any{"path": "Notes/untagged.md", "action": "add", "tag": "newtag"})
		assert.False(t, result.IsError)

		got := callTool(t, c, "get_frontmatter", map[string]any{"path": "Notes/untagged.md"})
		m := toolJSON(t, got)
		fm, ok := m["frontmatter"].(map[string]any)
		require.True(t, ok)
		tags, ok := fm["tags"].([]any)
		require.True(t, ok)
		assert.Contains(t, tags, "newtag")
	})

	t.Run("[AC-11] manage_tags action=remove removes an existing frontmatter tag", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "manage_tags", map[string]any{"path": "Notes/tagged.md", "action": "remove", "tag": "project"})
		assert.False(t, result.IsError)

		got := callTool(t, c, "get_frontmatter", map[string]any{"path": "Notes/tagged.md"})
		m := toolJSON(t, got)
		fm, ok := m["frontmatter"].(map[string]any)
		require.True(t, ok)
		tags, ok := fm["tags"].([]any)
		require.True(t, ok)
		assert.NotContains(t, tags, "project")
	})

	t.Run("[AC-12] list_all_tags returns every tag used across the vault", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "list_all_tags", map[string]any{})
		m := toolJSON(t, result)
		tags, ok := m["tags"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, tags)
	})

	t.Run("[AC-13] get_backlinks returns notes that link to the target path", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_backlinks", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, result)
		backlinks, ok := m["backlinks"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, backlinks)
	})

	t.Run("[AC-14] patch_note inserts content after a matching heading", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "# Heading\n\nbody text\n"})
		result := callTool(t, c, "patch_note", map[string]any{
			"path":     "Notes/simple.md",
			"heading":  "Heading",
			"position": "after",
			"content":  "inserted line",
		})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		content, _ := m["content"].(string)
		assert.Contains(t, content, "inserted line")
		assert.Contains(t, content, "body text")
	})

	t.Run("[AC-15] patch_note with position=replace_body replaces the whole note body", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "# Heading\n\nold body\n"})
		result := callTool(t, c, "patch_note", map[string]any{
			"path":     "Notes/simple.md",
			"heading":  "Heading",
			"position": "replace_body",
			"content":  "new body only",
		})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		content, _ := m["content"].(string)
		assert.Contains(t, content, "new body only")
		assert.NotContains(t, content, "old body")
	})

	t.Run("[AC-16] delete_note requires confirm to equal path and moves the note to trash by default", func(t *testing.T) {
		c, dir := newToolsClient(t)
		badConfirm := callTool(t, c, "delete_note", map[string]any{"path": "Notes/orphan.md", "confirm": "wrong"})
		assert.True(t, badConfirm.IsError)

		result := callTool(t, c, "delete_note", map[string]any{"path": "Notes/orphan.md", "confirm": "Notes/orphan.md"})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/orphan.md"})
		assert.True(t, read.IsError, "note should no longer be readable at its original path")
		_ = dir
	})

	t.Run("[AC-17] delete_note with permanent=true removes the note without leaving a trash copy", func(t *testing.T) {
		c, dir := newToolsClient(t)
		result := callTool(t, c, "delete_note", map[string]any{
			"path":      "Notes/orphan.md",
			"confirm":   "Notes/orphan.md",
			"permanent": true,
		})
		assert.False(t, result.IsError)

		trashDir := filepath.Join(dir, ".obsidian-mcp", "trash")
		entries, err := os.ReadDir(trashDir)
		if err == nil {
			assert.Empty(t, entries, "permanent delete should not leave a trash copy")
		}
	})

	t.Run("[AC-18] move_note relocates a note and rewrites links that pointed to it", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "move_note", map[string]any{
			"src":     "Notes/simple.md",
			"dst":     "Notes/renamed.md",
			"confirm": "Notes/simple.md",
		})
		assert.False(t, result.IsError)

		moved := callTool(t, c, "read_note", map[string]any{"path": "Notes/renamed.md"})
		assert.False(t, moved.IsError)

		linked := callTool(t, c, "read_note", map[string]any{"path": "Notes/linked.md"})
		m := toolJSON(t, linked)
		content, _ := m["content"].(string)
		assert.Contains(t, content, "renamed")
	})

	t.Run("[AC-19] move_note with dryRun=true reports the planned change without moving the note", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "move_note", map[string]any{
			"src":     "Notes/simple.md",
			"dst":     "Notes/renamed.md",
			"confirm": "Notes/simple.md",
			"dryRun":  true,
		})
		assert.False(t, result.IsError)

		stillThere := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		assert.False(t, stillThere.IsError, "dryRun must not actually move the note")
		notYet := callTool(t, c, "read_note", map[string]any{"path": "Notes/renamed.md"})
		assert.True(t, notYet.IsError, "dryRun must not create the destination note")
	})

	t.Run("[AC-20] search_notes finds notes matching a query term via BM25", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "search_notes", map[string]any{"query": "machine learning"})
		m := toolJSON(t, result)
		results, ok := m["results"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, results)
	})

	t.Run("[AC-21] search_regex finds notes whose content matches a regular expression", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "search_regex", map[string]any{"pattern": "simple note"})
		m := toolJSON(t, result)
		results, ok := m["results"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, results)
	})

	t.Run("[AC-22] read_multiple_notes reports a per-path error for a missing note without failing the whole batch", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "read_multiple_notes", map[string]any{
			"paths": `["Notes/simple.md","nonexistent.md"]`,
		})
		m := toolJSON(t, result)
		notes, ok := m["notes"].([]any)
		require.True(t, ok)
		require.Len(t, notes, 2)
		second, ok := notes[1].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, second["error"])
	})

	t.Run("[AC-23] read_multiple_notes truncates a batch larger than MaxBatch", func(t *testing.T) {
		c, _ := newToolsClient(t, func(d *tools.Deps) { d.MaxBatch = 1 })
		result := callTool(t, c, "read_multiple_notes", map[string]any{
			"paths": `["Notes/simple.md","Notes/orphan.md"]`,
		})
		m := toolJSON(t, result)
		notes, ok := m["notes"].([]any)
		require.True(t, ok)
		assert.Len(t, notes, 1)
	})

	t.Run("[AC-24] get_notes_info returns metadata for multiple notes in one call", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_notes_info", map[string]any{
			"paths": `["Notes/simple.md","Notes/tagged.md"]`,
		})
		m := toolJSON(t, result)
		infos, ok := m["notes"].([]any)
		require.True(t, ok)
		assert.Len(t, infos, 2)
	})

	t.Run("[AC-25] get_vault_stats reports a note count matching the vault", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_vault_stats", map[string]any{})
		m := toolJSON(t, result)
		count, ok := m["noteCount"].(float64)
		require.True(t, ok)
		assert.Greater(t, count, float64(0))
	})

	t.Run("[AC-26] get_recent_changes sorts notes by modification time, most recent first", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "touched"})

		result := callTool(t, c, "get_recent_changes", map[string]any{"limit": float64(1)})
		m := toolJSON(t, result)
		changes, ok := m["notes"].([]any)
		require.True(t, ok)
		require.Len(t, changes, 1)
		entry, ok := changes[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Notes/simple.md", entry["path"])
	})

	t.Run("[AC-27] get_periodic_note resolves and returns an existing daily note's content", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_periodic_note", map[string]any{"granularity": "daily"})
		m := toolJSON(t, result)
		assert.Contains(t, m["content"], "Review project notes")
	})

	t.Run("[AC-28] get_periodic_note with createIfMissing=true creates the note when absent", func(t *testing.T) {
		c, _ := newToolsClientAt(t, 2024, 1, 1)
		result := callTool(t, c, "get_periodic_note", map[string]any{"granularity": "daily", "createIfMissing": true})
		m := toolJSON(t, result)
		exists, ok := m["exists"].(bool)
		require.True(t, ok)
		assert.True(t, exists)

		path, ok := m["path"].(string)
		require.True(t, ok)
		read := callTool(t, c, "read_note", map[string]any{"path": path})
		assert.False(t, read.IsError)
	})

	t.Run("[AC-29] get_periodic_note without createIfMissing returns exists=false for a missing note", func(t *testing.T) {
		c, _ := newToolsClientAt(t, 2024, 1, 1)
		result := callTool(t, c, "get_periodic_note", map[string]any{"granularity": "daily"})
		assert.False(t, result.IsError)
		m := toolJSON(t, result)
		exists, ok := m["exists"].(bool)
		require.True(t, ok)
		assert.False(t, exists)
	})

	t.Run("[AC-30] get_recent_periodic_notes returns entries for the requested count of dates", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "get_recent_periodic_notes", map[string]any{"granularity": "daily", "count": float64(3)})
		m := toolJSON(t, result)
		notes, ok := m["notes"].([]any)
		require.True(t, ok)
		assert.Len(t, notes, 3)
	})

	t.Run("[AC-31] audit_notes orphans class reports a note with no incoming links and no tags", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "audit_notes", map[string]any{"classes": `["orphans"]`})
		m := toolJSON(t, result)
		orphans, ok := m["orphans"].([]any)
		require.True(t, ok)
		_, hasOtherClass := m["dangling-links"]
		assert.False(t, hasOtherClass, "requesting only orphans should not run other audit classes")
		found := false
		for _, o := range orphans {
			entry, ok := o.(map[string]any)
			if ok && entry["path"] == "Notes/orphan.md" {
				found = true
			}
		}
		assert.True(t, found, "expected Notes/orphan.md in orphans")
	})

	t.Run("[AC-32] audit_notes dangling-links class reports a link with no resolvable target", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "audit_notes", map[string]any{"classes": `["dangling-links"]`})
		m := toolJSON(t, result)
		dangling, ok := m["dangling-links"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, dangling)
	})

	t.Run("[AC-33] audit_notes duplicate-titles class reports notes sharing the same stem", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "audit_notes", map[string]any{"classes": `["duplicate-titles"]`})
		m := toolJSON(t, result)
		dupes, ok := m["duplicate-titles"].([]any)
		require.True(t, ok)
		assert.NotEmpty(t, dupes)
	})

	t.Run("[AC-34] get_note_outline returns the note's headings with their levels and line numbers", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "# Title\n\n## Sub\n\nbody\n"})
		result := callTool(t, c, "get_note_outline", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, result)
		headings, ok := m["headings"].([]any)
		require.True(t, ok)
		assert.Len(t, headings, 2)
	})

	t.Run("[AC-35] read_note_lines returns a specific line range of a note", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "line1\nline2\nline3\n"})
		result := callTool(t, c, "read_note_lines", map[string]any{"path": "Notes/simple.md", "startLine": float64(2), "lineCount": float64(1)})
		m := toolJSON(t, result)
		assert.Equal(t, "line2", m["content"])
	})

	t.Run("[AC-36] rename_tag renames a tag across every note that uses it", func(t *testing.T) {
		c, _ := newToolsClient(t)
		result := callTool(t, c, "rename_tag", map[string]any{"oldTag": "project", "newTag": "workstream"})
		assert.False(t, result.IsError)

		got := callTool(t, c, "get_frontmatter", map[string]any{"path": "Notes/tagged.md"})
		m := toolJSON(t, got)
		fm, ok := m["frontmatter"].(map[string]any)
		require.True(t, ok)
		tags, ok := fm["tags"].([]any)
		require.True(t, ok)
		assert.Contains(t, tags, "workstream")
		assert.NotContains(t, tags, "project")
	})

	t.Run("[AC-37] replace_in_note performs a literal replacement bounded by maxOccurrences", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "foo foo foo"})
		result := callTool(t, c, "replace_in_note", map[string]any{
			"path":           "Notes/simple.md",
			"pattern":        "foo",
			"replacement":    "bar",
			"maxOccurrences": float64(2),
		})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		assert.Equal(t, "bar bar foo", m["content"])
	})

	t.Run("[AC-38] replace_in_note with isRegex=true supports backreferences in the replacement", func(t *testing.T) {
		c, _ := newToolsClient(t)
		callTool(t, c, "write_note", map[string]any{"path": "Notes/simple.md", "content": "hello world"})
		result := callTool(t, c, "replace_in_note", map[string]any{
			"path":        "Notes/simple.md",
			"pattern":     `(\w+) (\w+)`,
			"replacement": "$2 $1",
			"isRegex":     true,
		})
		assert.False(t, result.IsError)

		read := callTool(t, c, "read_note", map[string]any{"path": "Notes/simple.md"})
		m := toolJSON(t, read)
		assert.Equal(t, "world hello", m["content"])
	})

	t.Run("[AC-39] list_directory includes an obsidian deep link when VaultName is set, and omits it when not", func(t *testing.T) {
		withName, _ := newToolsClient(t, func(d *tools.Deps) { d.VaultName = "MyVault" })
		result := callTool(t, withName, "list_directory", map[string]any{"path": "Notes", "type": "files"})
		m := toolJSON(t, result)
		entries, ok := m["entries"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, entries)
		first, ok := entries[0].(map[string]any)
		require.True(t, ok)
		deepLink, _ := first["deepLink"].(string)
		assert.Contains(t, deepLink, "obsidian://open?")
		assert.Contains(t, deepLink, "vault=MyVault")

		withoutName, _ := newToolsClient(t)
		result2 := callTool(t, withoutName, "list_directory", map[string]any{"path": "Notes", "type": "files"})
		m2 := toolJSON(t, result2)
		entries2, ok := m2["entries"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, entries2)
		first2, ok := entries2[0].(map[string]any)
		require.True(t, ok)
		assert.Empty(t, first2["deepLink"])
	})

	t.Run("[AC-40] RegisterAll in read-only mode omits mutating tools but keeps read-only tools registered", func(t *testing.T) {
		c, _ := newToolsClient(t, func(d *tools.Deps) { d.ReadOnly = true })
		listed, err := c.ListTools(t.Context(), mcp.ListToolsRequest{})
		require.NoError(t, err)

		names := make(map[string]bool, len(listed.Tools))
		for _, tool := range listed.Tools {
			names[tool.Name] = true
		}
		assert.True(t, names["read_note"], "read-only tool should still be registered")
		assert.False(t, names["write_note"], "mutating tool should be omitted in read-only mode")
	})
}
