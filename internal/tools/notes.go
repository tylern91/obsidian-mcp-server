package tools

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func readNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("read_note",
		mcp.WithDescription("Read a note from the vault. Returns content and metadata."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root (e.g. \"Notes/my-note.md\")"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	return newToolSpec(tool, readNoteHandler(deps))
}

func readNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		note, err := deps.Vault.ReadNote(ctx, path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type noteResponse struct {
			Path       string `json:"path"`
			Content    string `json:"content"`
			Size       int64  `json:"size"`
			ModTime    string `json:"modTime"` // RFC3339
			TokenCount int    `json:"tokenCount"`
		}

		resp := noteResponse{
			Path:       note.Path,
			Content:    note.Content,
			Size:       note.Size,
			ModTime:    note.ModTime.UTC().Format(time.RFC3339),
			TokenCount: response.CountTokens(note.Content),
		}

		return response.ToolResult(req, deps.PrettyPrint, resp)
	}
}

func writeNoteSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("write_note",
		mcp.WithDescription("Write or update a note in the vault."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write"),
		),
		mcp.WithString("mode",
			mcp.Description("Write mode: \"overwrite\" (default), \"append\", or \"prepend\""),
			mcp.Enum("overwrite", "append", "prepend"),
			mcp.DefaultString("overwrite"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	return newToolSpec(tool, writeNoteHandler(deps))
}

func writeNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		modeStr := req.GetString("mode", "overwrite")

		if err := deps.Vault.WriteNote(ctx, path, content, vault.WriteMode(modeStr)); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type writeResponse struct {
			Success bool   `json:"success"`
			Path    string `json:"path"`
			Mode    string `json:"mode"`
		}
		return response.ToolResult(req, deps.PrettyPrint, writeResponse{Success: true, Path: path, Mode: modeStr})
	}
}
