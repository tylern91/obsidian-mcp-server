package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
	"github.com/tylern91/obsidian-mcp-server/internal/resources"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newResourcesClient copies the fixture vault into a fresh temp dir, registers
// internal/resources against a real MCP server with a fixed-clock periodic
// service, and returns a client connected to it plus the temp vault root (so
// tests can mutate it, e.g. to exercise cache invalidation).
func newResourcesClient(t *testing.T, year, month, day int) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, copyDir("../../testdata/vault", dir))
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	deps := resources.Deps{
		Vault:    vault.New(dir, filter),
		Periodic: periodic.New(dir).WithClock(fixedClock(year, month, day)),
	}
	c := startClient(t, func(s *server.MCPServer) {
		resources.RegisterAll(s, deps)
	})
	return c, dir
}

func readResource(t *testing.T, c *client.Client, uri string) []mcp.ResourceContents {
	t.Helper()
	var req mcp.ReadResourceRequest
	req.Params.URI = uri
	result, err := c.ReadResource(t.Context(), req)
	require.NoError(t, err)
	return result.Contents
}

func resourceText(t *testing.T, contents []mcp.ResourceContents) mcp.TextResourceContents {
	t.Helper()
	require.NotEmpty(t, contents, "expected resource contents")
	tc, ok := contents[0].(mcp.TextResourceContents)
	require.True(t, ok, "expected TextResourceContents")
	return tc
}

func TestResources_Acceptance(t *testing.T) {
	t.Run("[AC-1] obsidian://vault/stats returns a JSON note count matching the vault", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://vault/stats"))
		assert.Equal(t, "application/json", tc.MIMEType)

		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(tc.Text), &m))
		count, ok := m["noteCount"].(float64)
		require.True(t, ok, "expected numeric noteCount")
		assert.Greater(t, count, float64(0))
	})

	t.Run("[AC-2] obsidian://vault/tags returns a JSON tag list from the vault", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://vault/tags"))

		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(tc.Text), &m))
		tags, ok := m["tags"].([]any)
		require.True(t, ok, "expected tags array")
		assert.NotEmpty(t, tags)
	})

	t.Run("[AC-3] obsidian://note/{path} returns the raw markdown content of an existing note", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://note/Notes/simple.md"))
		assert.Equal(t, "text/markdown", tc.MIMEType)
		assert.Contains(t, tc.Text, "This is a simple note with no frontmatter.")
	})

	t.Run("[AC-4] obsidian://note/{path} returns an error payload, not a tool error, for a missing note", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://note/nonexistent.md"))
		assert.Contains(t, tc.Text, "not found")
	})

	t.Run("[AC-5] obsidian://backlinks/{path} lists notes that link to the target", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://backlinks/Notes/simple.md"))

		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(tc.Text), &m))
		backlinks, ok := m["backlinks"].([]any)
		require.True(t, ok, "expected backlinks array")
		require.NotEmpty(t, backlinks, "expected at least one backlink from Daily Notes/2024-01-15.md")
		entry, ok := backlinks[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Daily Notes/2024-01-15.md", entry["path"])
	})

	t.Run("[AC-6] obsidian://periodic/{granularity} resolves and returns the current daily note's content", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)
		tc := resourceText(t, readResource(t, c, "obsidian://periodic/daily"))
		assert.Equal(t, "text/markdown", tc.MIMEType)
		assert.Contains(t, tc.Text, "Review project notes")
	})

	t.Run("[AC-7] obsidian://periodic/{granularity} returns an explanatory placeholder when the note doesn't exist yet", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 1)
		tc := resourceText(t, readResource(t, c, "obsidian://periodic/daily"))
		assert.Contains(t, tc.Text, "does not exist yet")
	})

	t.Run("[AC-8] the vault/stats resource cache invalidates when the vault root mtime changes", func(t *testing.T) {
		c, root := newResourcesClient(t, 2024, 1, 15)

		var before map[string]any
		require.NoError(t, json.Unmarshal([]byte(resourceText(t, readResource(t, c, "obsidian://vault/stats")).Text), &before))
		countBefore := before["noteCount"].(float64)

		newNote := filepath.Join(root, "extra-note.md")
		require.NoError(t, os.WriteFile(newNote, []byte("# Extra\n"), 0o644))
		require.NoError(t, os.Chtimes(root, time.Now().Add(time.Minute), time.Now().Add(time.Minute)))

		var after map[string]any
		require.NoError(t, json.Unmarshal([]byte(resourceText(t, readResource(t, c, "obsidian://vault/stats")).Text), &after))
		countAfter := after["noteCount"].(float64)

		assert.Equal(t, countBefore+1, countAfter, "cache should reflect the newly added note after root mtime changes")
	})

	t.Run("[AC-9] RegisterAll registers all five vault resources with the MCP server", func(t *testing.T) {
		c, _ := newResourcesClient(t, 2024, 1, 15)

		resourcesResult, err := c.ListResources(t.Context(), mcp.ListResourcesRequest{})
		require.NoError(t, err)
		templatesResult, err := c.ListResourceTemplates(t.Context(), mcp.ListResourceTemplatesRequest{})
		require.NoError(t, err)

		assert.Len(t, resourcesResult.Resources, 2, "expected vault/stats and vault/tags as static resources")
		assert.Len(t, templatesResult.ResourceTemplates, 3, "expected note, periodic, and backlinks as resource templates")
	})
}
