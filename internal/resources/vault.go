package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

const resourceCacheTTL = 30 * time.Second

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
	cache := &resourceCache{ttl: resourceCacheTTL}
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return cache.get(deps.Vault.Root(), func() ([]mcp.ResourceContents, error) {
			return computeVaultStats(ctx, deps)
		})
	}
}

func computeVaultStats(ctx context.Context, deps Deps) ([]mcp.ResourceContents, error) {
	type statsResult struct {
		NoteCount  int      `json:"noteCount"`
		TotalBytes int64    `json:"totalBytes"`
		TotalLinks int      `json:"totalLinks"`
		TopTags    []string `json:"topTags"`
		VaultRoot  string   `json:"vaultRoot"`
	}

	vs, err := deps.Vault.VaultStats(ctx, vault.VaultStatsOpts{})
	if err != nil {
		return response.ErrorResourceContents("obsidian://vault/stats", fmt.Sprintf("vault walk failed: %v", err)), nil
	}

	topEntries := vault.TopTagsByCount(vs.TagCounts, 10)
	top := make([]string, 0, len(topEntries))
	for _, tc := range topEntries {
		top = append(top, fmt.Sprintf("%s (%d)", tc.Name, tc.Count))
	}

	stats := statsResult{
		NoteCount:  vs.NoteCount,
		TotalBytes: vs.TotalBytes,
		TotalLinks: vs.TotalLinks,
		TopTags:    top,
		VaultRoot:  deps.Vault.Root(),
	}
	return response.JSONResourceContents("obsidian://vault/stats", "application/json", stats, deps.PrettyPrint), nil
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
	cache := &resourceCache{ttl: resourceCacheTTL}
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return cache.get(deps.Vault.Root(), func() ([]mcp.ResourceContents, error) {
			return computeVaultTags(ctx, deps)
		})
	}
}

func computeVaultTags(ctx context.Context, deps Deps) ([]mcp.ResourceContents, error) {
	tagCounts, err := deps.Vault.AggregateTags(ctx)
	if err != nil {
		return response.ErrorResourceContents("obsidian://vault/tags", fmt.Sprintf("tag aggregation failed: %v", err)), nil
	}

	entries := vault.TopTagsByCount(tagCounts, 0)

	type tagsResult struct {
		Tags  []vault.TagCount `json:"tags"`
		Total int              `json:"total"`
	}
	return response.JSONResourceContents("obsidian://vault/tags", "application/json", tagsResult{Tags: entries, Total: len(entries)}, deps.PrettyPrint), nil
}

// pathFromURI extracts the path component after the given prefix from a resource URI.
// e.g. pathFromURI("obsidian://note/Notes/foo.md", "obsidian://note/") → "Notes/foo.md"
func pathFromURI(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return uri[len(prefix):]
}
