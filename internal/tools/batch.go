package tools

import (
	"context"
	"fmt"
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
		mcp.WithNumber("headChars",
			mcp.Description("Number of runes for headOf when summary=true (default: 200)"),
			mcp.DefaultNumber(200),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, readMultipleNotesHandler(deps))
}

func readMultipleNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var paths []string
		if errResult := parseJSONArg(req, "paths", &paths); errResult != nil {
			return errResult, nil
		}

		summary := req.GetBool("summary", false)
		headChars := req.GetInt("headChars", defaultHeadChars)
		if headChars <= 0 {
			headChars = defaultHeadChars
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
		var paths []string
		if errResult := parseJSONArg(req, "paths", &paths); errResult != nil {
			return errResult, nil
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

		vaultOpts := vault.VaultStatsOpts{
			IncludeTokens: includeTokenCounts,
		}
		if includeTokenCounts {
			vaultOpts.TokenCounter = response.CountTokens
		}
		vs, err := deps.Vault.VaultStats(ctx, vaultOpts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("vault stats: %v", err)), nil
		}

		top20Tags := vault.TopTagsByCount(vs.TagCounts, 20)

		type noteRef struct {
			Path    string `json:"path"`
			ModTime string `json:"modTime"`
		}

		type vaultStatsResponse struct {
			NoteCount   int              `json:"noteCount"`
			TotalBytes  int64            `json:"totalBytes"`
			TotalLinks  int              `json:"totalLinks"`
			TotalTags   int              `json:"totalTags"`
			TopTags     []vault.TagCount `json:"topTags"`
			OldestNote  *noteRef         `json:"oldestNote,omitempty"`
			NewestNote  *noteRef         `json:"newestNote,omitempty"`
			TotalTokens *int             `json:"totalTokens,omitempty"`
			VaultRoot   string           `json:"vaultRoot"`
		}

		statsResp := vaultStatsResponse{
			NoteCount:  vs.NoteCount,
			TotalBytes: vs.TotalBytes,
			TotalLinks: vs.TotalLinks,
			TotalTags:  len(vs.TagCounts),
			TopTags:    top20Tags,
			VaultRoot:  deps.Vault.Root(),
		}
		if vs.Oldest != nil {
			statsResp.OldestNote = &noteRef{Path: vs.Oldest.Path, ModTime: vs.Oldest.ModTime.UTC().Format(time.RFC3339)}
		}
		if vs.Newest != nil {
			statsResp.NewestNote = &noteRef{Path: vs.Newest.Path, ModTime: vs.Newest.ModTime.UTC().Format(time.RFC3339)}
		}
		if includeTokenCounts {
			toks := vs.TotalTokens
			statsResp.TotalTokens = &toks
		}

		out, err := response.FormatJSON(statsResp, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}
