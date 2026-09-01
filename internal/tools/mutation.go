package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func patchNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("patch_note",
		mcp.WithDescription("Apply a heading-anchored patch to a note. Insert content before or after a heading, or replace the heading's body."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("heading",
			mcp.Required(),
			mcp.Description("Heading text to anchor the patch (without the # prefix, e.g. \"Introduction\")"),
		),
		mcp.WithString("position",
			mcp.Required(),
			mcp.Description("Where to apply the patch relative to the heading"),
			mcp.Enum("before", "after", "replace_body"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to insert or use as the replacement body"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, patchNoteHandler(deps))
}

func patchNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		heading, err := req.RequireString("heading")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		position, err := req.RequireString("position")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if err := deps.Vault.PatchNote(ctx, path, vault.PatchOp{
			Heading:  heading,
			Position: position,
			Content:  content,
		}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type patchResponse struct {
			Success  bool   `json:"success"`
			Path     string `json:"path"`
			Heading  string `json:"heading"`
			Position string `json:"position"`
		}
		return response.ToolResult(req, deps.PrettyPrint, patchResponse{
			Success:  true,
			Path:     path,
			Heading:  heading,
			Position: position,
		})
	}
}

func deleteNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("delete_note",
		mcp.WithDescription("Delete a note from the vault. By default it moves the note to .obsidian-mcp/trash (recoverable); pass permanent=true to hard-delete instead. Requires confirm to match path exactly as a safety guard, regardless of permanent."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("confirm",
			mcp.Required(),
			mcp.Description("Must match path exactly to confirm the deletion"),
		),
		mcp.WithBoolean("permanent",
			mcp.Description("Hard-delete instead of moving to trash (default: false)"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, deleteNoteHandler(deps))
}

func deleteNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		confirm, err := req.RequireString("confirm")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		permanent := req.GetBool("permanent", false)

		if err := deps.Vault.DeleteNote(ctx, path, confirm, permanent); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type deleteResponse struct {
			Success   bool   `json:"success"`
			Path      string `json:"path"`
			Permanent bool   `json:"permanent"`
		}
		return response.ToolResult(req, deps.PrettyPrint, deleteResponse{Success: true, Path: path, Permanent: permanent})
	}
}

func moveNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("move_note",
		mcp.WithDescription("Move or rename a note within the vault. Creates intermediate directories as needed. Requires confirm to match src exactly. Note: confirm guards the source path only; verify dst carefully before submitting. By default also rewrites inbound links elsewhere in the vault to point at the new location — ambiguous link targets are reported, never guessed at, and left unchanged."),
		mcp.WithString("src",
			mcp.Required(),
			mcp.Description("Source path of the note relative to the vault root"),
		),
		mcp.WithString("dst",
			mcp.Required(),
			mcp.Description("Destination path relative to the vault root"),
		),
		mcp.WithString("confirm",
			mcp.Required(),
			mcp.Description("Must match src exactly to confirm the move"),
		),
		mcp.WithBoolean("updateLinks",
			mcp.Description("Rewrite inbound links to src elsewhere in the vault so they point at dst (default: true)"),
		),
		mcp.WithBoolean("dryRun",
			mcp.Description("Preview only — no file is moved and no links are rewritten (default: false)"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, moveNoteHandler(deps))
}

func moveNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		src, err := req.RequireString("src")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dst, err := req.RequireString("dst")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		confirm, err := req.RequireString("confirm")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		updateLinks := req.GetBool("updateLinks", true)
		dryRun := req.GetBool("dryRun", false)

		result, err := deps.Vault.MoveNote(ctx, src, dst, confirm, updateLinks, dryRun)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type linkRewriteEntry struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			OldText   string `json:"oldText"`
			NewText   string `json:"newText,omitempty"`
			Ambiguous bool   `json:"ambiguous"`
		}
		links := make([]linkRewriteEntry, len(result.Links))
		for i, l := range result.Links {
			links[i] = linkRewriteEntry{Path: l.Path, Line: l.Line, OldText: l.OldText, NewText: l.NewText, Ambiguous: l.Ambiguous}
		}

		type moveResponse struct {
			Success bool               `json:"success"`
			Src     string             `json:"src"`
			Dst     string             `json:"dst"`
			Moved   bool               `json:"moved"`
			Links   []linkRewriteEntry `json:"links,omitempty"`
		}
		return response.ToolResult(req, deps.PrettyPrint, moveResponse{
			Success: true,
			Src:     result.Src,
			Dst:     result.Dst,
			Moved:   result.Moved,
			Links:   links,
		})
	}
}
