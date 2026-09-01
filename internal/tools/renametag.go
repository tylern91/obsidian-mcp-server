package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func renameTagSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("rename_tag",
		mcp.WithDescription("Rename a tag across every note in the vault, in both frontmatter and inline occurrences."),
		mcp.WithString("oldTag",
			mcp.Required(),
			mcp.Description("Tag to rename (without the '#' prefix)"),
		),
		mcp.WithString("newTag",
			mcp.Required(),
			mcp.Description("New tag name (without the '#' prefix)"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, renameTagHandler(deps))
}

func renameTagHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		oldTag, err := req.RequireString("oldTag")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newTag, err := req.RequireString("newTag")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		renames, err := deps.Vault.RenameTag(ctx, oldTag, newTag)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type renameEntry struct {
			Path           string `json:"path"`
			FrontmatterHit bool   `json:"frontmatterHit"`
			InlineCount    int    `json:"inlineCount"`
		}
		entries := make([]renameEntry, len(renames))
		for i, r := range renames {
			entries[i] = renameEntry{Path: r.Path, FrontmatterHit: r.FrontmatterHit, InlineCount: r.InlineCount}
		}

		type renameResponse struct {
			OldTag       string        `json:"oldTag"`
			NewTag       string        `json:"newTag"`
			NotesChanged []renameEntry `json:"notesChanged"`
			Total        int           `json:"total"`
		}
		return response.ToolResult(req, deps.PrettyPrint, renameResponse{
			OldTag:       oldTag,
			NewTag:       newTag,
			NotesChanged: entries,
			Total:        len(entries),
		})
	}
}
