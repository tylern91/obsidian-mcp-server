package tools

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

const defaultMaxBatch = 10
const defaultHeadChars = 200

func registerReadMultipleNotes(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("read_multiple_notes",
		mcp.WithDescription("Read the content of multiple notes in a single request"),
		mcp.WithString("paths",
			mcp.Required(),
			mcp.Description(`JSON array of note paths relative to the vault root (e.g. ["Notes/a.md","Notes/b.md"])`),
		),
		mcp.WithBoolean("summary",
			mcp.Description("When true, return headOf instead of full content (default: false)"),
		),
		mcp.WithString("headChars",
			mcp.Description("Number of runes for headOf when summary=true (default: 200)"),
			mcp.DefaultString("200"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, readMultipleNotesHandler(deps))
}

func readMultipleNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pathsJSON, err := req.RequireString("paths")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var paths []string
		if err := json.Unmarshal([]byte(pathsJSON), &paths); err != nil {
			return mcp.NewToolResultError("paths: invalid JSON array: " + err.Error()), nil
		}

		summary := req.GetBool("summary", false)

		headCharsStr := req.GetString("headChars", "200")
		headChars := defaultHeadChars
		if n, parseErr := strconv.Atoi(headCharsStr); parseErr == nil && n > 0 {
			headChars = n
		}

		maxBatch := deps.MaxBatch
		if maxBatch <= 0 {
			maxBatch = defaultMaxBatch
		}

		truncated := false
		if len(paths) > maxBatch {
			paths = paths[:maxBatch]
			truncated = true
		}

		type noteEntry struct {
			Path       string  `json:"path"`
			Size       int64   `json:"size,omitempty"`
			ModTime    string  `json:"modTime,omitempty"`
			TokenCount int     `json:"tokenCount,omitempty"`
			Content    *string `json:"content,omitempty"`
			HeadOf     *string `json:"headOf,omitempty"`
			Error      *string `json:"error,omitempty"`
		}

		notes := make([]noteEntry, 0, len(paths))
		for _, p := range paths {
			note, readErr := deps.Vault.ReadNote(ctx, p)
			if readErr != nil {
				errStr := readErr.Error()
				notes = append(notes, noteEntry{Path: p, Error: &errStr})
				continue
			}

			entry := noteEntry{
				Path:       note.Path,
				Size:       note.Size,
				ModTime:    note.ModTime.UTC().Format(time.RFC3339),
				TokenCount: response.CountTokens(note.Content),
			}
			if summary {
				head := response.HeadRunes(note.Content, headChars)
				entry.HeadOf = &head
			} else {
				c := note.Content
				entry.Content = &c
			}
			notes = append(notes, entry)
		}

		type batchResponse struct {
			Notes     []noteEntry `json:"notes"`
			Count     int         `json:"count"`
			Truncated bool        `json:"truncated"`
		}

		result, err := response.FormatJSON(batchResponse{
			Notes:     notes,
			Count:     len(notes),
			Truncated: truncated,
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerGetNotesInfo(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_notes_info",
		mcp.WithDescription("Get metadata for multiple notes without reading full content"),
		mcp.WithString("paths",
			mcp.Required(),
			mcp.Description(`JSON array of note paths relative to the vault root (e.g. ["Notes/a.md","Notes/b.md"])`),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, getNotesInfoHandler(deps))
}

func getNotesInfoHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pathsJSON, err := req.RequireString("paths")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var paths []string
		if err := json.Unmarshal([]byte(pathsJSON), &paths); err != nil {
			return mcp.NewToolResultError("paths: invalid JSON array: " + err.Error()), nil
		}

		maxBatch := deps.MaxBatch
		if maxBatch <= 0 {
			maxBatch = defaultMaxBatch
		}

		truncated := false
		if len(paths) > maxBatch {
			paths = paths[:maxBatch]
			truncated = true
		}

		type infoEntry struct {
			Path      string  `json:"path"`
			Size      int64   `json:"size,omitempty"`
			ModTime   string  `json:"modTime,omitempty"`
			Title     string  `json:"title,omitempty"`
			TagCount  int     `json:"tagCount"`
			LinkCount int     `json:"linkCount"`
			Error     *string `json:"error,omitempty"`
		}

		notes := make([]infoEntry, 0, len(paths))
		for _, p := range paths {
			info, statErr := deps.Vault.StatNote(ctx, p)
			if statErr != nil {
				errStr := statErr.Error()
				notes = append(notes, infoEntry{Path: p, Error: &errStr})
				continue
			}

			notes = append(notes, infoEntry{
				Path:      info.Path,
				Size:      info.Size,
				ModTime:   info.ModTime.UTC().Format(time.RFC3339),
				Title:     info.Title,
				TagCount:  info.TagCount,
				LinkCount: info.LinkCount,
			})
		}

		type infoResponse struct {
			Notes     []infoEntry `json:"notes"`
			Count     int         `json:"count"`
			Truncated bool        `json:"truncated"`
		}

		result, err := response.FormatJSON(infoResponse{
			Notes:     notes,
			Count:     len(notes),
			Truncated: truncated,
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerGetVaultStats(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_vault_stats",
		mcp.WithDescription("Get aggregate statistics about the entire vault"),
		mcp.WithBoolean("includeTokenCounts",
			mcp.Description("When true, also sum token counts across all notes (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, getVaultStatsHandler(deps))
}

func getVaultStatsHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		includeTokenCounts := req.GetBool("includeTokenCounts", false)

		// Gather tag map (unique tag → count across all notes).
		tagMap, err := deps.Vault.AggregateTags(ctx)
		if err != nil {
			return mcp.NewToolResultError("AggregateTags: " + err.Error()), nil
		}

		type noteRef struct {
			Path    string `json:"path"`
			ModTime string `json:"modTime"`
		}

		var (
			noteCount  int
			totalBytes int64
			totalLinks int
			totalToks  int

			oldest *noteRef
			newest *noteRef
		)

		walkErr := deps.Vault.WalkNotes(ctx, func(rel, abs string) error {
			data, readErr := os.ReadFile(abs)
			if readErr != nil {
				// Skip unreadable files silently.
				return nil
			}

			info, statErr := os.Stat(abs)
			if statErr != nil {
				return nil
			}

			noteCount++
			totalBytes += info.Size()
			content := string(data)

			links := vault.ExtractLinks(content)
			totalLinks += len(links)

			if includeTokenCounts {
				totalToks += response.CountTokens(content)
			}

			mt := info.ModTime()
			ref := &noteRef{
				Path:    rel,
				ModTime: mt.UTC().Format(time.RFC3339),
			}

			if oldest == nil || mt.Before(parseModTime(oldest.ModTime)) {
				oldest = ref
			}
			if newest == nil || mt.After(parseModTime(newest.ModTime)) {
				newest = ref
			}

			return nil
		})
		if walkErr != nil {
			return mcp.NewToolResultError("WalkNotes: " + walkErr.Error()), nil
		}

		// Build top-20 tags sorted descending by count, then alphabetically.
		type tagEntry struct {
			Tag   string `json:"tag"`
			Count int    `json:"count"`
		}
		topTags := make([]tagEntry, 0, len(tagMap))
		for tag, count := range tagMap {
			topTags = append(topTags, tagEntry{Tag: tag, Count: count})
		}
		sort.Slice(topTags, func(i, j int) bool {
			if topTags[i].Count != topTags[j].Count {
				return topTags[i].Count > topTags[j].Count
			}
			return topTags[i].Tag < topTags[j].Tag
		})
		if len(topTags) > 20 {
			topTags = topTags[:20]
		}

		type vaultStatsResponse struct {
			NoteCount   int        `json:"noteCount"`
			TotalBytes  int64      `json:"totalBytes"`
			TotalLinks  int        `json:"totalLinks"`
			TotalTags   int        `json:"totalTags"`
			TopTags     []tagEntry `json:"topTags"`
			OldestNote  *noteRef   `json:"oldestNote,omitempty"`
			NewestNote  *noteRef   `json:"newestNote,omitempty"`
			TotalTokens *int       `json:"totalTokens,omitempty"`
			VaultRoot   string     `json:"vaultRoot"`
		}

		statsResp := vaultStatsResponse{
			NoteCount:  noteCount,
			TotalBytes: totalBytes,
			TotalLinks: totalLinks,
			TotalTags:  len(tagMap),
			TopTags:    topTags,
			OldestNote: oldest,
			NewestNote: newest,
			VaultRoot:  deps.Vault.Root(),
		}
		if includeTokenCounts {
			statsResp.TotalTokens = &totalToks
		}

		out, err := response.FormatJSON(statsResp, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}

// parseModTime parses an RFC3339 time string, returning zero time on failure.
func parseModTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
