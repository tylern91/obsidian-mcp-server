package tools

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func registerListDirectory(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("list_directory",
		mcp.WithDescription("List files and directories in the vault. Pass an empty path to list the vault root."),
		mcp.WithString("path",
			mcp.Description("Directory path relative to vault root. Empty string lists the vault root."),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, listDirectoryHandler(deps))
}

func listDirectoryHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := req.GetString("path", "")
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)

		entries, err := deps.Vault.ListDirectory(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type dirEntry struct {
			Name    string `json:"name"`
			Path    string `json:"path"`
			IsDir   bool   `json:"isDir"`
			Size    int64  `json:"size"`
			ModTime string `json:"modTime"`
		}
		type dirResponse struct {
			Path    string     `json:"path"`
			Entries []dirEntry `json:"entries"`
		}

		resp := dirResponse{Path: path, Entries: make([]dirEntry, len(entries))}
		for i, e := range entries {
			resp.Entries[i] = dirEntry{
				Name:    e.Name,
				Path:    e.Path,
				IsDir:   e.IsDir,
				Size:    e.Size,
				ModTime: e.ModTime.UTC().Format(time.RFC3339),
			}
		}

		result, err := response.FormatJSON(resp, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

// ListDirectoryHandler returns the list_directory handler for testing.
func ListDirectoryHandler(deps Deps) server.ToolHandlerFunc { return listDirectoryHandler(deps) }
