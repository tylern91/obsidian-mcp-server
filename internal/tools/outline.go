package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

// defaultLineCount and maxLineCount bound read_note_lines. These are
// independent of deps.MaxResults — that ceiling governs result/entry counts
// for search and listing tools, a different unit than "lines of one note".
const (
	defaultLineCount = 200
	maxLineCount     = 2000
)

func getNoteOutlineSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("get_note_outline",
		mcp.WithDescription("Return a note's heading tree (level, text, line number) without its body — cheaper than read_note when only structure is needed."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	return newToolSpec(tool, getNoteOutlineHandler(deps))
}

func getNoteOutlineHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		headings, err := deps.Vault.GetNoteOutline(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type headingEntry struct {
			Level int    `json:"level"`
			Text  string `json:"text"`
			Line  int    `json:"line"`
		}
		entries := make([]headingEntry, len(headings))
		for i, h := range headings {
			entries[i] = headingEntry{Level: h.Level, Text: h.Text, Line: h.Line}
		}

		type outlineResponse struct {
			Path     string         `json:"path"`
			Headings []headingEntry `json:"headings"`
		}
		return response.ToolResult(req, deps.PrettyPrint, outlineResponse{Path: path, Headings: entries})
	}
}

func readNoteLinesSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("read_note_lines",
		mcp.WithDescription("Read a bounded range of lines from a note, starting at startLine. Cheaper than read_note for a long note when only a section is needed."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithNumber("startLine",
			mcp.Required(),
			mcp.Description("1-indexed line number to start reading from"),
		),
		mcp.WithNumber("lineCount",
			mcp.Description("Maximum number of lines to return (default 200, capped at 2000)"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	return newToolSpec(tool, readNoteLinesHandler(deps))
}

func readNoteLinesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		startLine, err := req.RequireInt("startLine")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		lineCount := effectiveLimit(req.GetInt("lineCount", 0), defaultLineCount, maxLineCount)

		result, err := deps.Vault.ReadNoteLines(ctx, path, startLine, lineCount)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type linesResponse struct {
			Path       string `json:"path"`
			StartLine  int    `json:"startLine"`
			EndLine    int    `json:"endLine"`
			TotalLines int    `json:"totalLines"`
			Content    string `json:"content"`
		}
		return response.ToolResult(req, deps.PrettyPrint, linesResponse{
			Path:       result.Path,
			StartLine:  result.StartLine,
			EndLine:    result.EndLine,
			TotalLines: result.TotalLines,
			Content:    result.Content,
		})
	}
}
