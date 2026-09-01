package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func getFrontmatterSpec(deps Deps) toolSpec {
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
	return newToolSpec(tool, getFrontmatterHandler(deps))
}

func getFrontmatterHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		fm, body, err := deps.Vault.GetFrontmatter(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type fmResponse struct {
			Path        string         `json:"path"`
			Frontmatter map[string]any `json:"frontmatter"`
			Body        string         `json:"body"`
		}
		return response.ToolResult(req, deps.PrettyPrint, fmResponse{
			Path:        path,
			Frontmatter: fm,
			Body:        body,
		})
	}
}

func updateFrontmatterSpec(deps Deps) toolSpec {
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
		mcp.WithString("if_match",
			mcp.Description("Optional etag from a prior read; the update fails with REVISION_CONFLICT if the note's current content does not match"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, updateFrontmatterHandler(deps))
}

func updateFrontmatterHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		updates, errResult := optStringMap(req, "updates")
		if errResult != nil {
			return errResult, nil
		}

		removeKeys, errResult := optStringSlice(req, "removeKeys")
		if errResult != nil {
			return errResult, nil
		}

		ifMatch := req.GetString("if_match", "")

		if err := deps.Vault.UpdateFrontmatter(ctx, path, updates, removeKeys, ifMatchOpt(ifMatch)...); err != nil {
			return vaultWriteError(err), nil
		}

		type updateResponse struct {
			Success bool     `json:"success"`
			Path    string   `json:"path"`
			Updated []string `json:"updated,omitempty"`
			Removed []string `json:"removed,omitempty"`
		}
		updatedKeys := make([]string, 0, len(updates))
		for k := range updates {
			updatedKeys = append(updatedKeys, k)
		}
		return response.ToolResult(req, deps.PrettyPrint, updateResponse{
			Success: true,
			Path:    path,
			Updated: updatedKeys,
			Removed: removeKeys,
		})
	}
}
