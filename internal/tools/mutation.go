package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func registerPatchNote(s *server.MCPServer, deps Deps) {
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
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	s.AddTool(tool, patchNoteHandler(deps))
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
		result, err := response.FormatJSON(patchResponse{
			Success:  true,
			Path:     path,
			Heading:  heading,
			Position: position,
		}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerDeleteNote(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("delete_note",
		mcp.WithDescription("Permanently delete a note from the vault. Requires confirm to match path exactly as a safety guard."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("confirm",
			mcp.Required(),
			mcp.Description("Must match path exactly to confirm the deletion"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	s.AddTool(tool, deleteNoteHandler(deps))
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

		if err := deps.Vault.DeleteNote(ctx, path, confirm); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type deleteResponse struct {
			Success bool   `json:"success"`
			Path    string `json:"path"`
		}
		result, err := response.FormatJSON(deleteResponse{Success: true, Path: path}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerMoveNote(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("move_note",
		mcp.WithDescription("Move or rename a note within the vault. Creates intermediate directories as needed. Requires confirm to match src exactly. Note: confirm guards the source path only; verify dst carefully before submitting."),
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
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	s.AddTool(tool, moveNoteHandler(deps))
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

		if err := deps.Vault.MoveNote(ctx, src, dst, confirm); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type moveResponse struct {
			Success bool   `json:"success"`
			Src     string `json:"src"`
			Dst     string `json:"dst"`
		}
		result, err := response.FormatJSON(moveResponse{Success: true, Src: src, Dst: dst}, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
