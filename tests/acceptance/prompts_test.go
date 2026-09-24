package acceptance_test

import (
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
	"github.com/tylern91/obsidian-mcp-server/internal/prompts"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newPromptsClient copies the fixture vault into a fresh temp dir, registers
// internal/prompts against a real MCP server with a fixed-clock periodic
// service, and returns a client connected to it over the real MCP protocol.
func newPromptsClient(t *testing.T, year, month, day int) *client.Client {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, copyDir("../../testdata/vault", dir))
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	deps := prompts.Deps{
		Vault:    vault.New(dir, filter),
		Periodic: periodic.New(dir).WithClock(fixedClock(year, month, day)),
	}
	return startClient(t, func(s *server.MCPServer) {
		prompts.RegisterAll(s, deps)
	})
}

func getPrompt(t *testing.T, c *client.Client, name string, args map[string]string) *mcp.GetPromptResult {
	t.Helper()
	var req mcp.GetPromptRequest
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := c.GetPrompt(t.Context(), req)
	require.NoError(t, err)
	return result
}

func promptText(t *testing.T, result *mcp.GetPromptResult) string {
	t.Helper()
	require.NotEmpty(t, result.Messages, "expected at least one prompt message")
	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text
}

func TestPrompts_Acceptance(t *testing.T) {
	t.Run("[AC-1] summarize_note builds a prompt embedding the note's path and content", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "summarize_note", map[string]string{"path": "Notes/simple.md"}))
		assert.Contains(t, text, "Notes/simple.md")
		assert.Contains(t, text, "This is a simple note with no frontmatter.")
	})

	t.Run("[AC-2] summarize_note reports an error in the prompt body when path is missing", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "summarize_note", nil))
		assert.Contains(t, text, "Error")
	})

	t.Run("[AC-3] summarize_note reports an error in the prompt body for a nonexistent note", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "summarize_note", map[string]string{"path": "nonexistent/note.md"}))
		assert.Contains(t, text, "Error")
	})

	t.Run("[AC-4] find_related builds a prompt including the note's content and its frontmatter tags", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "find_related", map[string]string{"path": "Notes/tagged.md"}))
		assert.Contains(t, text, "Notes/tagged.md")
		assert.Contains(t, text, "Tags: project")
	})

	t.Run("[AC-5] find_related reports an error in the prompt body when path is missing", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "find_related", nil))
		assert.Contains(t, text, "Error")
	})

	t.Run("[AC-6] vault_health_check reports orphaned and untagged notes present in the vault", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "vault_health_check", nil))
		assert.Contains(t, text, "notes total")
		assert.Contains(t, text, "Orphaned")
		assert.Contains(t, text, "Untagged")
	})

	t.Run("[AC-7] daily_note_review reports an error in the prompt body for a non-integer offset", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "daily_note_review", map[string]string{"offset": "not-a-number"}))
		assert.Contains(t, text, "Error")
	})

	t.Run("[AC-8] daily_note_review resolves today's daily note and includes its content in the prompt", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "daily_note_review", nil))
		assert.Contains(t, text, "2024-01-15")
		assert.Contains(t, text, "Review project notes")
	})

	t.Run("[AC-9] weekly_review rejects a positive weekOffset as an error in the prompt body", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "weekly_review", map[string]string{"weekOffset": "1"}))
		assert.Contains(t, text, "Error")
		assert.Contains(t, text, "0 (this week) or negative")
	})

	t.Run("[AC-10] weekly_review includes the current week's daily note content in the prompt", func(t *testing.T) {
		c := newPromptsClient(t, 2024, 1, 15)
		text := promptText(t, getPrompt(t, c, "weekly_review", nil))
		assert.Contains(t, text, "2024-01-15")
		assert.Contains(t, text, "Review project notes")
	})
}
