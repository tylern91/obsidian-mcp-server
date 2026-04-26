package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func registerGetFrontmatter(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_frontmatter",
		mcp.WithDescription("Read the YAML frontmatter of a note. Returns key-value pairs and the note body."),
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
	s.AddTool(tool, getFrontmatterHandler(deps))
}

func getFrontmatterHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)

		fm, body, err := deps.Vault.GetFrontmatter(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type fmResponse struct {
			Path        string         `json:"path"`
			Frontmatter map[string]any `json:"frontmatter"`
			Body        string         `json:"body"`
		}
		result, err := response.FormatJSON(fmResponse{
			Path:        path,
			Frontmatter: fm,
			Body:        body,
		}, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerUpdateFrontmatter(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("update_frontmatter",
		mcp.WithDescription("Update YAML frontmatter fields in a note. Preserves existing key ordering. Use updates to set/overwrite keys and removeKeys to delete keys."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("updates",
			mcp.Description("JSON object of key-value pairs to set or overwrite in the frontmatter"),
		),
		mcp.WithString("removeKeys",
			mcp.Description("JSON array of key names to remove from the frontmatter"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	s.AddTool(tool, updateFrontmatterHandler(deps))
}

func updateFrontmatterHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var updates map[string]any
		if updatesStr := req.GetString("updates", ""); updatesStr != "" {
			if err := json.Unmarshal([]byte(updatesStr), &updates); err != nil {
				return mcp.NewToolResultError("updates: invalid JSON object: " + err.Error()), nil
			}
		}

		var removeKeys []string
		if removeStr := req.GetString("removeKeys", ""); removeStr != "" {
			if err := json.Unmarshal([]byte(removeStr), &removeKeys); err != nil {
				return mcp.NewToolResultError("removeKeys: invalid JSON array: " + err.Error()), nil
			}
		}

		if err := deps.Vault.UpdateFrontmatter(ctx, path, updates, removeKeys); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type updateResponse struct {
			Success    bool     `json:"success"`
			Path       string   `json:"path"`
			Updated    []string `json:"updated,omitempty"`
			Removed    []string `json:"removed,omitempty"`
		}
		updatedKeys := make([]string, 0, len(updates))
		for k := range updates {
			updatedKeys = append(updatedKeys, k)
		}
		result, err := response.FormatJSON(updateResponse{
			Success: true,
			Path:    path,
			Updated: updatedKeys,
			Removed: removeKeys,
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
