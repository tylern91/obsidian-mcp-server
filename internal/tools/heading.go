package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

// headingLineArg reads the optional 1-based "line" selector shared by
// patch_note and rename_heading. Zero (absent) means no selector.
func headingLineArg(req mcp.CallToolRequest) (int, *mcp.CallToolResult) {
	line := req.GetInt("line", 0)
	if line < 0 {
		return 0, mcp.NewToolResultError("line must be a positive 1-based line number")
	}
	return line, nil
}

func renameHeadingSpec(deps Deps) toolSpec {
	tool := mcp.NewTool("rename_heading",
		mcp.WithDescription("Rename a heading in a note, keeping its level. By default also rewrites anchors that point at the old heading ([[Note#Old]], [[Note#Old|alias]], [text](Note.md#old)) vault-wide, so they keep resolving; ambiguous link targets are reported, never guessed at, and left unchanged. Headings inside fenced code blocks are ignored. If several headings share the text, the call fails with AMBIGUOUS_HEADING listing each match; pass line to choose one."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("heading",
			mcp.Required(),
			mcp.Description("Current heading text (without the # prefix)"),
		),
		mcp.WithString("newHeading",
			mcp.Required(),
			mcp.Description("New heading text (without the # prefix); the heading level is kept"),
		),
		mcp.WithNumber("line",
			mcp.Description("Optional 1-based line number of the heading (as returned by get_note_outline), to disambiguate duplicates; the line must hold a heading whose text matches"),
		),
		mcp.WithBoolean("updateLinks",
			mcp.Description("Rewrite anchors pointing at the old heading elsewhere in the vault (default: true)"),
		),
		mcp.WithString("if_match",
			mcp.Description("Optional etag from a prior read; the rename fails with REVISION_CONFLICT if the note's current content does not match"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	)
	return newToolSpec(tool, renameHeadingHandler(deps))
}

func renameHeadingHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		heading, err := req.RequireString("heading")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newHeading, err := req.RequireString("newHeading")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		line, errResult := headingLineArg(req)
		if errResult != nil {
			return errResult, nil
		}
		updateLinks := req.GetBool("updateLinks", true)
		ifMatch := req.GetString("if_match", "")

		result, err := deps.Vault.RenameHeading(ctx, path, heading, newHeading, line, updateLinks, ifMatchOpt(ifMatch)...)
		if err != nil && result == nil {
			return vaultWriteError(err), nil
		}

		type linkEntry struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			OldText   string `json:"oldText"`
			NewText   string `json:"newText,omitempty"`
			Ambiguous bool   `json:"ambiguous"`
		}
		links := make([]linkEntry, len(result.Links))
		for i, l := range result.Links {
			links[i] = linkEntry{Path: l.Path, Line: l.Line, OldText: l.OldText, NewText: l.NewText, Ambiguous: l.Ambiguous}
		}

		type renameResponse struct {
			Success      bool        `json:"success"`
			Path         string      `json:"path"`
			Heading      string      `json:"heading"`
			NewHeading   string      `json:"newHeading"`
			Level        int         `json:"level"`
			Line         int         `json:"line"`
			Links        []linkEntry `json:"links,omitempty"`
			LinksSkipped string      `json:"linksSkipped,omitempty"`

			// Set only when the heading was renamed but link rewriting failed part-way.
			LinksComplete    *bool  `json:"linksComplete,omitempty"`
			LinkRewriteError string `json:"linkRewriteError,omitempty"`
		}
		resp := renameResponse{
			Success: true, Path: result.Path, Heading: result.OldHeading, NewHeading: result.NewHeading,
			Level: result.Level, Line: result.Line, Links: links, LinksSkipped: result.LinksSkipped,
		}
		if err != nil {
			incomplete := false
			resp.LinksComplete = &incomplete
			resp.LinkRewriteError = err.Error()
		}
		return response.ToolResult(req, deps.PrettyPrint, resp)
	}
}
