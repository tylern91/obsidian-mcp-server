package tools

import (
	"context"
	"sort"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// defaultMaxDirEntries caps listing output when no explicit limit is set.
const defaultMaxDirEntries = 50

func listDirectorySpec(deps Deps) toolSpec {
	tool := mcp.NewTool("list_directory",
		mcp.WithDescription("List files and directories in the vault. "+
			"Supports glob filtering, file/dir type filtering, limit+offset pagination, "+
			"and a concise mode (default) that omits size and modTime for compact output. "+
			"Pass an empty path to list the vault root."),
		mcp.WithString("path",
			mcp.Description("Directory path relative to vault root. Empty string lists the vault root."),
		),
		mcp.WithString("filter",
			mcp.Description("Glob pattern applied to entry names (e.g. '*.md', '2026-*'). Empty string matches all entries."),
		),
		mcp.WithString("type",
			mcp.Description("Restrict listing to 'files', 'dirs', or 'all' (default)."),
			mcp.Enum("all", "files", "dirs"),
			mcp.DefaultString("all"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return. 0 uses the server default (50)."),
		),
		mcp.WithNumber("offset",
			mcp.Description("Number of entries to skip (for pagination). Applied after sort and filter."),
		),
		mcp.WithBoolean("concise",
			mcp.Description("When true (default), omit size and modTime to reduce output size. Set false to include full metadata."),
			mcp.DefaultBool(true),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	return newToolSpec(tool, listDirectoryHandler(deps))
}

func listDirectoryHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		filter := req.GetString("filter", "")
		typeFilter := req.GetString("type", "all")
		limit := req.GetInt("limit", 0)
		offset := req.GetInt("offset", 0)
		concise := req.GetBool("concise", true)

		limit = effectiveLimit(limit, defaultMaxDirEntries, deps.MaxResults)
		if offset < 0 {
			offset = 0
		}

		raw, err := deps.Vault.ListDirectory(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// ── 1. Type filter ──────────────────────────────────────────────────
		entries := raw[:0]
		for _, e := range raw {
			switch typeFilter {
			case "files":
				if e.IsDir {
					continue
				}
			case "dirs":
				if !e.IsDir {
					continue
				}
			}
			entries = append(entries, e)
		}

		// ── 2. Glob filter ──────────────────────────────────────────────────
		if filter != "" {
			matched := entries[:0]
			for _, e := range entries {
				if search.MatchesPathScope(e.Name, filter) {
					matched = append(matched, e)
				}
			}
			entries = matched
		}

		// ── 3. Sort (lexical by name — ensures stable offset pagination) ────
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})

		// ── 4. Record total before pagination ───────────────────────────────
		total := len(entries)

		// ── 5. Offset ───────────────────────────────────────────────────────
		if offset >= len(entries) {
			entries = entries[:0]
		} else {
			entries = entries[offset:]
		}

		// ── 6. Limit / truncation ───────────────────────────────────────────
		truncated := false
		if len(entries) > limit {
			entries = entries[:limit]
			truncated = true
		}

		// ── 7. Build response ───────────────────────────────────────────────
		type conciseEntry struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			IsDir    bool   `json:"isDir"`
			DeepLink string `json:"deepLink,omitempty"`
		}
		type fullEntry struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			IsDir    bool   `json:"isDir"`
			Size     int64  `json:"size"`
			ModTime  string `json:"modTime"`
			DeepLink string `json:"deepLink,omitempty"`
		}

		// deepLinkFor omits the link for directories — obsidian://open resolves
		// to a file, not a folder.
		deepLinkFor := func(e vault.DirEntry) string {
			if e.IsDir {
				return ""
			}
			return obsidianDeepLink(deps.VaultName, e.Path)
		}

		var respEntries any
		if concise {
			es := make([]conciseEntry, len(entries))
			for i, e := range entries {
				es[i] = conciseEntry{Name: e.Name, Path: e.Path, IsDir: e.IsDir, DeepLink: deepLinkFor(e)}
			}
			respEntries = es
		} else {
			es := make([]fullEntry, len(entries))
			for i, e := range entries {
				es[i] = fullEntry{
					Name:     e.Name,
					Path:     e.Path,
					IsDir:    e.IsDir,
					Size:     e.Size,
					ModTime:  e.ModTime.UTC().Format(time.RFC3339),
					DeepLink: deepLinkFor(e),
				}
			}
			respEntries = es
		}

		resp := struct {
			Path      string `json:"path"`
			Entries   any    `json:"entries"`
			Count     int    `json:"count"`
			Total     int    `json:"total"`
			Truncated bool   `json:"truncated"`
		}{
			Path:      path,
			Entries:   respEntries,
			Count:     len(entries),
			Total:     total,
			Truncated: truncated,
		}

		return response.ToolResult(req, deps.PrettyPrint, resp)
	}
}
