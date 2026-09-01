package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func replaceInNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("replace_in_note",
		mcp.WithDescription("Scoped search-and-replace within a single note. Supports literal or RE2 regex matching (regex replacement may use $1-style backreferences) and an occurrence cap."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("Text or RE2 regex pattern to search for"),
		),
		mcp.WithString("replacement",
			mcp.Required(),
			mcp.Description("Replacement text (may use $1-style backreferences when isRegex is true)"),
		),
		mcp.WithBoolean("isRegex",
			mcp.Description("Treat pattern as a regex instead of a literal string (default false)"),
		),
		mcp.WithNumber("maxOccurrences",
			mcp.Description("Maximum number of occurrences to replace, in document order. 0 or omitted means unbounded."),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, replaceInNoteHandler(deps))
}

func replaceInNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pattern, err := req.RequireString("pattern")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		replacement, err := req.RequireString("replacement")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		isRegex := req.GetBool("isRegex", false)
		maxOccurrences := req.GetInt("maxOccurrences", 0)

		result, err := deps.Vault.ReplaceInNote(ctx, path, pattern, replacement, isRegex, maxOccurrences)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type replaceResponse struct {
			Path             string `json:"path"`
			OccurrencesFound int    `json:"occurrencesFound"`
			Replaced         int    `json:"replaced"`
		}
		return response.ToolResult(req, deps.PrettyPrint, replaceResponse{
			Path:             result.Path,
			OccurrencesFound: result.OccurrencesFound,
			Replaced:         result.Replaced,
		})
	}
}
