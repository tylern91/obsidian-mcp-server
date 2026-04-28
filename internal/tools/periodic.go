package tools

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

const defaultPeriodicCount = 5

func registerGetPeriodicNote(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_periodic_note",
		mcp.WithDescription("Get a periodic note (daily, weekly, monthly, quarterly, or yearly)"),
		mcp.WithString("granularity",
			mcp.Required(),
			mcp.Description("Periodic note type"),
			mcp.Enum("daily", "weekly", "monthly", "quarterly", "yearly"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Offset from current period: 0=current, -1=previous, +1=next (default: 0)"),
			mcp.DefaultNumber(0),
		),
		mcp.WithBoolean("createIfMissing",
			mcp.Description("Create the note if it does not exist (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, getPeriodicNoteHandler(deps))
}

func getPeriodicNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		granularity, err := req.RequireString("granularity")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		offset := req.GetInt("offset", 0)
		createIfMissing := req.GetBool("createIfMissing", false)

		resolvedPath, err := deps.Periodic.Resolve(granularity, offset)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		note, readErr := deps.Vault.ReadNote(ctx, resolvedPath)
		if readErr != nil {
			if createIfMissing {
				if writeErr := deps.Vault.WriteNote(ctx, resolvedPath, "", vault.WriteModeOverwrite); writeErr != nil {
					return mcp.NewToolResultError("create periodic note: " + writeErr.Error()), nil
				}
				note, readErr = deps.Vault.ReadNote(ctx, resolvedPath)
				if readErr != nil {
					return mcp.NewToolResultError("read after create: " + readErr.Error()), nil
				}
			} else {
				type notFoundResponse struct {
					Exists bool   `json:"exists"`
					Path   string `json:"path"`
				}
				result, jsonErr := response.FormatJSON(notFoundResponse{Exists: false, Path: resolvedPath}, deps.PrettyPrint)
				if jsonErr != nil {
					return mcp.NewToolResultError(jsonErr.Error()), nil
				}
				return mcp.NewToolResultText(result), nil
			}
		}

		type periodicNoteResponse struct {
			Exists     bool   `json:"exists"`
			Path       string `json:"path"`
			Content    string `json:"content"`
			Size       int64  `json:"size"`
			ModTime    string `json:"modTime"`
			TokenCount int    `json:"tokenCount"`
		}

		resp := periodicNoteResponse{
			Exists:     true,
			Path:       note.Path,
			Content:    note.Content,
			Size:       note.Size,
			ModTime:    note.ModTime.UTC().Format(time.RFC3339),
			TokenCount: response.CountTokens(note.Content),
		}

		result, err := response.FormatJSON(resp, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerGetRecentPeriodicNotes(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_recent_periodic_notes",
		mcp.WithDescription("Get the N most recent periodic notes"),
		mcp.WithString("granularity",
			mcp.Required(),
			mcp.Description("Periodic note type"),
			mcp.Enum("daily", "weekly", "monthly", "quarterly", "yearly"),
		),
		mcp.WithNumber("count",
			mcp.Description("Number of recent notes to return (default: 5)"),
			mcp.DefaultNumber(5),
		),
		mcp.WithBoolean("summary",
			mcp.Description("When true, return headOf (200 chars) instead of full content (default: true)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, getRecentPeriodicNotesHandler(deps))
}

func getRecentPeriodicNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		granularity, err := req.RequireString("granularity")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		count := req.GetInt("count", defaultPeriodicCount)
		if count <= 0 {
			count = defaultPeriodicCount
		}

		maxResults := deps.MaxResults
		if maxResults <= 0 {
			maxResults = 20
		}
		if count > maxResults {
			count = maxResults
		}

		// summary defaults to true when not provided
		summary := req.GetBool("summary", true)

		dates, err := deps.Periodic.RecentDates(granularity, count)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type noteEntry struct {
			Date    string  `json:"date"`
			Path    string  `json:"path"`
			Exists  bool    `json:"exists"`
			Size    *int64  `json:"size,omitempty"`
			ModTime *string `json:"modTime,omitempty"`
			Tokens  *int    `json:"tokenCount,omitempty"`
			Content *string `json:"content,omitempty"`
			HeadOf  *string `json:"headOf,omitempty"`
		}

		notes := make([]noteEntry, 0, len(dates))
		for i, d := range dates {
			dateStr := d.Format("2006-01-02")

			// Resolve path for offset -i (i=0 is current, i=1 is previous, etc.)
			resolvedPath, resolveErr := deps.Periodic.Resolve(granularity, -i)
			if resolveErr != nil {
				return mcp.NewToolResultError("resolve: " + resolveErr.Error()), nil
			}

			entry := noteEntry{
				Date:   dateStr,
				Path:   resolvedPath,
				Exists: false,
			}

			note, readErr := deps.Vault.ReadNote(ctx, resolvedPath)
			if readErr == nil {
				entry.Exists = true
				sz := note.Size
				mt := note.ModTime.UTC().Format(time.RFC3339)
				tc := response.CountTokens(note.Content)
				entry.Size = &sz
				entry.ModTime = &mt
				entry.Tokens = &tc

				if summary {
					head := response.HeadRunes(note.Content, defaultHeadChars)
					entry.HeadOf = &head
				} else {
					c := note.Content
					entry.Content = &c
				}
			}

			notes = append(notes, entry)
		}

		type recentResponse struct {
			Notes []noteEntry `json:"notes"`
			Count int         `json:"count"`
		}

		result, err := response.FormatJSON(recentResponse{
			Notes: notes,
			Count: len(notes),
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
