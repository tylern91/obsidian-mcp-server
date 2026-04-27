package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
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
