package tools

import (
	"context"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

const defaultRecentLimit = 10

func registerGetRecentChanges(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_recent_changes",
		mcp.WithDescription("List notes most recently modified in the vault"),
		mcp.WithString("limit",
			mcp.Description("Maximum number of notes to return (default: 10)"),
			mcp.DefaultString("10"),
		),
		mcp.WithString("since",
			mcp.Description("Only include notes modified on or after this date (ISO-8601, e.g. \"2024-01-01\")"),
		),
		mcp.WithString("summary",
			mcp.Description("When false, include the first 200 characters of each note (default: true)"),
			mcp.DefaultString("true"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, recentChangesHandler(deps))
}

func recentChangesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse limit.
		limitStr := req.GetString("limit", "10")
		limit := defaultRecentLimit
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
		maxResults := deps.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if limit > maxResults {
			limit = maxResults
		}

		// Parse since.
		sinceStr := req.GetString("since", "")
		var sinceTime time.Time
		if sinceStr != "" {
			var parseErr error
			sinceTime, parseErr = time.Parse("2006-01-02", sinceStr)
			if parseErr != nil {
				return mcp.NewToolResultError("since: invalid date format, expected ISO-8601 (e.g. \"2024-01-01\"): " + parseErr.Error()), nil
			}
		}

		// Parse summary flag (string "true"/"false", default true).
		summaryStr := req.GetString("summary", "true")
		summary := summaryStr != "false"

		// Collect all entries via WalkNotes.
		type entry struct {
			rel     string
			abs     string
			modTime time.Time
		}
		var entries []entry

		walkErr := deps.Vault.WalkNotes(ctx, func(rel, abs string) error {
			info, err := os.Stat(abs)
			if err != nil {
				return nil // skip unreadable files
			}
			mt := info.ModTime().UTC()
			// Apply since filter.
			if !sinceTime.IsZero() && mt.Before(sinceTime) {
				return nil
			}
			entries = append(entries, entry{rel: rel, abs: abs, modTime: mt})
			return nil
		})
		if walkErr != nil {
			return mcp.NewToolResultError("walk vault: " + walkErr.Error()), nil
		}

		// Sort descending by modTime (newest first).
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].modTime.After(entries[j].modTime)
		})

		// Truncate to limit.
		if len(entries) > limit {
			entries = entries[:limit]
		}

		// Build response.
		type noteEntry struct {
			Path    string  `json:"path"`
			ModTime string  `json:"modTime"`
			HeadOf  *string `json:"headOf,omitempty"`
		}

		notes := make([]noteEntry, 0, len(entries))
		for _, e := range entries {
			n := noteEntry{
				Path:    e.rel,
				ModTime: e.modTime.Format(time.RFC3339),
			}
			if !summary {
				data, readErr := os.ReadFile(e.abs)
				if readErr == nil {
					head := response.HeadRunes(string(data), 200)
					n.HeadOf = &head
				}
			}
			notes = append(notes, n)
		}

		type recentResponse struct {
			Notes []noteEntry `json:"notes"`
			Count int         `json:"count"`
		}

		out, err := response.FormatJSON(recentResponse{
			Notes: notes,
			Count: len(notes),
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}
