package resources

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func registerVaultStats(s *server.MCPServer, deps Deps) {
	res := mcp.NewResource(
		"obsidian://vault/stats",
		"Vault statistics",
		mcp.WithResourceDescription("Aggregate statistics for the entire Obsidian vault: note count, total size, link count, top tags."),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(res, vaultStatsHandler(deps))
}

func vaultStatsHandler(deps Deps) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		type statsResult struct {
			NoteCount  int      `json:"noteCount"`
			TotalBytes int64    `json:"totalBytes"`
			TotalLinks int      `json:"totalLinks"`
			TopTags    []string `json:"topTags"`
			VaultRoot  string   `json:"vaultRoot"`
		}

		var noteCount int
		var totalBytes int64

		err := deps.Vault.WalkNotes(ctx, func(rel, _ string) error {
			note, err := deps.Vault.ReadNote(ctx, rel)
			if err != nil {
				return nil
			}
			noteCount++
			totalBytes += note.Size
			return nil
		})
		if err != nil {
			return resourceError("obsidian://vault/stats", fmt.Sprintf("vault walk failed: %v", err)), nil
		}

		tagCounts, err := deps.Vault.AggregateTags(ctx)
		if err != nil {
			tagCounts = map[string]int{}
		}

		type tagEntry struct {
			name  string
			count int
		}
		var entries []tagEntry
		for name, count := range tagCounts {
			entries = append(entries, tagEntry{name, count})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].count != entries[j].count {
				return entries[i].count > entries[j].count
			}
			return entries[i].name < entries[j].name
		})
		top := make([]string, 0, 10)
		for i, e := range entries {
			if i >= 10 {
				break
			}
			top = append(top, fmt.Sprintf("%s (%d)", e.name, e.count))
		}

		stats := statsResult{
			NoteCount:  noteCount,
			TotalBytes: totalBytes,
			TopTags:    top,
			VaultRoot:  deps.Vault.Root(),
		}
		text, err := response.FormatJSON(stats, deps.PrettyPrint)
		if err != nil {
			return resourceError("obsidian://vault/stats", err.Error()), nil
		}
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      "obsidian://vault/stats",
			MIMEType: "application/json",
			Text:     text,
		}}, nil
	}
}

func registerVaultTags(s *server.MCPServer, deps Deps) {
	res := mcp.NewResource(
		"obsidian://vault/tags",
		"Vault tag index",
		mcp.WithResourceDescription("All tags used in the vault with their note counts, sorted by frequency."),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(res, vaultTagsHandler(deps))
}

func vaultTagsHandler(deps Deps) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		tagCounts, err := deps.Vault.AggregateTags(ctx)
		if err != nil {
			return resourceError("obsidian://vault/tags", fmt.Sprintf("tag aggregation failed: %v", err)), nil
		}

		type tagEntry struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		var entries []tagEntry
		for name, count := range tagCounts {
			entries = append(entries, tagEntry{name, count})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Count != entries[j].Count {
				return entries[i].Count > entries[j].Count
			}
			return entries[i].Name < entries[j].Name
		})

		type tagsResult struct {
			Tags  []tagEntry `json:"tags"`
			Total int        `json:"total"`
		}
		text, err := response.FormatJSON(tagsResult{Tags: entries, Total: len(entries)}, deps.PrettyPrint)
		if err != nil {
			return resourceError("obsidian://vault/tags", err.Error()), nil
		}
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      "obsidian://vault/tags",
			MIMEType: "application/json",
			Text:     text,
		}}, nil
	}
}

// resourceError returns a JSON error payload as a resource result.
func resourceError(uri, msg string) []mcp.ResourceContents {
	body := fmt.Sprintf(`{"error":%s}`, jsonString(msg))
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI:      uri,
		MIMEType: "application/json",
		Text:     body,
	}}
}

// jsonString returns a JSON-encoded string literal.
func jsonString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

// pathFromURI extracts the path component after the given prefix from a resource URI.
// e.g. pathFromURI("obsidian://note/Notes/foo.md", "obsidian://note/") → "Notes/foo.md"
func pathFromURI(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return filepath.FromSlash(uri[len(prefix):])
}
