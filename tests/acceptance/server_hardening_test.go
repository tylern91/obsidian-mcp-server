package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// newFilteredToolsClient is newToolsClient with a caller-supplied ignore list.
func newFilteredToolsClient(t *testing.T, ignore []string, opts ...func(*tools.Deps)) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, copyDir("../../testdata/vault", dir))
	vs := vault.New(dir, vault.NewPathFilter(ignore, []string{".md", ".markdown", ".txt", ".canvas"}))
	deps := tools.Deps{Vault: vs, Search: search.New(vs), MaxBatch: 10, MaxResults: 50}
	for _, opt := range opts {
		opt(&deps)
	}
	c := startClient(t, func(s *server.MCPServer) { tools.RegisterAll(s, deps) })
	return c, dir
}

func writeVaultFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644))
}

func TestServerHardening_Acceptance(t *testing.T) {
	t.Run("[AC-1] an ignore entry containing a slash is a root-relative, NFC, case-insensitive prefix on component boundaries, including through a symlink", func(t *testing.T) {
		ignore := []string{".obsidian", "Journal/Private", "Café/Secret"}
		c, dir := newFilteredToolsClient(t, ignore)
		writeVaultFile(t, dir, "Journal/Private/x.md", "private")
		writeVaultFile(t, dir, "Journal/PrivateStuff/y.md", "public y")
		writeVaultFile(t, dir, "Other/Journal/Private/z.md", "public z")
		writeVaultFile(t, dir, norm.NFD.String("Café/Secret")+"/n.md", "nfd secret")
		if err := os.Symlink(filepath.Join(dir, "Journal", "Private", "x.md"), filepath.Join(dir, "Notes", "alias.md")); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}

		for _, p := range []string{"Journal/Private/x.md", "journal/private/x.md", "Notes/alias.md", norm.NFC.String("Café/Secret") + "/n.md", norm.NFD.String("Café/Secret") + "/n.md"} {
			assert.True(t, callTool(t, c, "read_note", map[string]any{"path": p}).IsError, "%q must be ignored", p)
		}
		for _, p := range []string{"Journal/PrivateStuff/y.md", "Other/Journal/Private/z.md"} {
			assert.False(t, callTool(t, c, "read_note", map[string]any{"path": p}).IsError, "%q must stay readable", p)
		}

		res := callTool(t, c, "search_regex", map[string]any{"pattern": "private|nfd secret", "limit": 50})
		assert.NotContains(t, toolText(t, res), "Journal/Private/x.md")
		assert.NotContains(t, toolText(t, res), "n.md")
	})

	t.Run("[AC-2] config rejects ignore entries that are '.', '..' or contain a '..' component", func(t *testing.T) {
		vaultDir := t.TempDir()
		for _, bad := range []string{"..", ".", "a/../b", "../x", "/"} {
			_, err := config.Load([]string{"--vault", vaultDir, "--ignore", bad})
			require.Error(t, err, "entry %q must be rejected", bad)
			assert.Contains(t, err.Error(), "ignore")
		}
		cfg, err := config.Load([]string{"--vault", vaultDir, "--ignore", ".git,Journal/Private"})
		require.NoError(t, err)
		assert.Equal(t, []string{".git", "Journal/Private"}, cfg.IgnorePatterns)
	})

	t.Run("[AC-3] search_notes reports matchCount and truncated when the limit cut results", func(t *testing.T) {
		c, _ := newToolsClient(t)
		cut := toolJSON(t, callTool(t, c, "search_notes", map[string]any{"query": "machine learning", "limit": 1}))
		assert.Equal(t, true, cut["truncated"])
		assert.Equal(t, float64(1), cut["total"])
		assert.Greater(t, cut["matchCount"], float64(1))

		full := toolJSON(t, callTool(t, c, "search_notes", map[string]any{"query": "machine learning", "limit": 50}))
		assert.Equal(t, false, full["truncated"])
		assert.Equal(t, full["total"], full["matchCount"])
	})

	t.Run("[AC-4] search_regex reports matchCount and truncated when the limit cut results", func(t *testing.T) {
		c, _ := newToolsClient(t)
		cut := toolJSON(t, callTool(t, c, "search_regex", map[string]any{"pattern": "learning", "limit": 1}))
		assert.Equal(t, true, cut["truncated"])
		assert.Equal(t, float64(1), cut["total"])
		assert.Greater(t, cut["matchCount"], float64(1))

		full := toolJSON(t, callTool(t, c, "search_regex", map[string]any{"pattern": "learning", "limit": 50}))
		assert.Equal(t, false, full["truncated"])
		assert.Equal(t, full["total"], full["matchCount"])
	})

	t.Run("[AC-5] list_all_tags pages with limit/offset in count-desc then name order and reports total and truncated", func(t *testing.T) {
		c, _ := newToolsClient(t)
		all := toolJSON(t, callTool(t, c, "list_all_tags", map[string]any{}))
		allTags, _ := all["tags"].([]any)
		total := int(all["total"].(float64))
		require.Greater(t, total, 3)
		assert.Len(t, allTags, total, "default returns every tag")
		assert.Equal(t, false, all["truncated"])

		var paged []any
		for off := 0; off < total; off += 2 {
			p := toolJSON(t, callTool(t, c, "list_all_tags", map[string]any{"limit": 2, "offset": off}))
			assert.Equal(t, float64(total), p["total"])
			tags, _ := p["tags"].([]any)
			paged = append(paged, tags...)
			assert.Equal(t, off+len(tags) < total, p["truncated"])
		}
		assert.Equal(t, allTags, paged)

		for i := 1; i < len(allTags); i++ {
			a, b := allTags[i-1].(map[string]any), allTags[i].(map[string]any)
			if a["count"] == b["count"] {
				assert.Less(t, a["name"], b["name"])
			} else {
				assert.Greater(t, a["count"], b["count"])
			}
		}

		past := toolJSON(t, callTool(t, c, "list_all_tags", map[string]any{"offset": total + 5}))
		assert.Empty(t, past["tags"])
		assert.Equal(t, false, past["truncated"])
	})

	t.Run("[AC-6] the initialize response carries the server instructions", func(t *testing.T) {
		srv := server.NewMCPServer("t", "0.0.0", server.WithInstructions(tools.Instructions))
		c, err := client.NewInProcessClient(srv)
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
		require.NoError(t, c.Start(t.Context()))
		var req mcp.InitializeRequest
		req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		res, err := c.Initialize(t.Context(), req)
		require.NoError(t, err)
		for _, want := range []string{"read_note", "if_match", "get_note_outline", ".obsidian-mcp/trash", "untrusted", "truncated"} {
			assert.Contains(t, res.Instructions, want)
		}
	})

	t.Run("[AC-7] --tool-timeout / OBSIDIAN_TOOL_TIMEOUT default to off, honor flag over env, and reject negative or malformed values", func(t *testing.T) {
		vaultDir := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vaultDir})
		require.NoError(t, err)
		assert.Zero(t, cfg.ToolTimeout)

		t.Setenv("OBSIDIAN_TOOL_TIMEOUT", "45s")
		cfg, err = config.Load([]string{"--vault", vaultDir})
		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, cfg.ToolTimeout)

		cfg, err = config.Load([]string{"--vault", vaultDir, "--tool-timeout", "2s"})
		require.NoError(t, err)
		assert.Equal(t, 2*time.Second, cfg.ToolTimeout)

		_, err = config.Load([]string{"--vault", vaultDir, "--tool-timeout", "-1s"})
		assert.Error(t, err)
		t.Setenv("OBSIDIAN_TOOL_TIMEOUT", "soon")
		_, err = config.Load([]string{"--vault", vaultDir})
		assert.Error(t, err)
	})

	t.Run("[AC-8] a tool call that outlives the tool timeout returns a clear tool error; with no timeout it succeeds", func(t *testing.T) {
		c, _ := newFilteredToolsClient(t, []string{".obsidian"}, func(d *tools.Deps) { d.ToolTimeout = time.Nanosecond })
		res := callTool(t, c, "search_regex", map[string]any{"pattern": "learning"})
		assert.True(t, res.IsError)
		assert.Contains(t, toolText(t, res), "timed out")
		assert.Contains(t, toolText(t, res), "search_regex")

		c2, _ := newFilteredToolsClient(t, []string{".obsidian"})
		assert.False(t, callTool(t, c2, "search_regex", map[string]any{"pattern": "learning"}).IsError)
	})
}

func TestPathBypass_Acceptance(t *testing.T) {
	t.Run("[AC-9] unclean paths cannot bypass a root-anchored ignore entry on read, write, list, patch, delete or move", func(t *testing.T) {
		dir := t.TempDir()
		writeVaultFile(t, dir, "Journal/Private/x.md", "# H\nsecret\n")
		writeVaultFile(t, dir, "Other/ok.md", "# H\nok\n")
		vs := vault.New(dir, vault.NewPathFilter([]string{"Journal/Private"}, []string{".md"}))
		ctx := t.Context()

		for _, p := range []string{
			"Other/../Journal/Private/x.md",
			"./Journal/Private/x.md",
			"Journal//Private/x.md",
			"Other/..//Journal/Private/x.md",
			".\\Journal\\Private\\x.md",
			"Other\\..\\Journal\\Private\\x.md",
		} {
			_, err := vs.ReadNote(ctx, p)
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "read %q", p)
			err = vs.WriteNote(ctx, p, "pwned", vault.WriteModeOverwrite)
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "write %q", p)
			err = vs.PatchNote(ctx, p, vault.PatchOp{Heading: "H", Position: "after", Content: "x"})
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "patch %q", p)
			err = vs.DeleteNote(ctx, p, p, false)
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "delete %q", p)
			_, err = vs.MoveNote(ctx, p, "Other/moved.md", p, false, false)
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "move src %q", p)
			_, err = vs.MoveNote(ctx, "Other/ok.md", p, "Other/ok.md", false, false)
			assert.ErrorIs(t, err, vault.ErrPathRestricted, "move dst %q", p)
		}
		_, err := vs.ListDirectory(ctx, "Other/../Journal/Private")
		assert.ErrorIs(t, err, vault.ErrPathRestricted, "list")
		_, err = vs.ListDirectory(ctx, "./Journal//Private")
		assert.ErrorIs(t, err, vault.ErrPathRestricted, "list dotted")

		got, err := os.ReadFile(filepath.Join(dir, "Journal/Private/x.md"))
		require.NoError(t, err)
		assert.Equal(t, "# H\nsecret\n", string(got))
		assert.FileExists(t, filepath.Join(dir, "Other/ok.md"))

		// The filter itself cleans its input, whatever the caller passes.
		f := vault.NewPathFilter([]string{"Journal/Private"}, nil)
		assert.True(t, f.IsIgnored("Other/../Journal/Private/x.md"))
		assert.False(t, f.IsIgnored("Journal/Other/../PrivateStuff/y.md"))
	})
}
