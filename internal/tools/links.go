package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func registerGetBacklinks(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_backlinks",
		mcp.WithDescription("Find all notes in the vault that link to the specified note."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the target note relative to the vault root"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, getBacklinksHandler(deps))
}

func getBacklinksHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)

		backlinks, err := deps.Vault.GetBacklinks(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type backlinkEntry struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		}
		entries := make([]backlinkEntry, 0, len(backlinks))
		for _, bl := range backlinks {
			entries = append(entries, backlinkEntry{
				Path:    bl.Path,
				Line:    bl.Line,
				Snippet: bl.Snippet,
			})
		}

		type backlinksResponse struct {
			Target    string          `json:"target"`
			Backlinks []backlinkEntry `json:"backlinks"`
			Total     int             `json:"total"`
		}
		result, err := response.FormatJSON(backlinksResponse{
			Target:    path,
			Backlinks: entries,
			Total:     len(entries),
		}, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
