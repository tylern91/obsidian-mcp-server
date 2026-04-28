package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func registerFindRelated(s *server.MCPServer, deps Deps) {
	prompt := mcp.NewPrompt("find_related",
		mcp.WithPromptDescription("Suggest related notes worth linking to/from the given note, grouped by relationship type."),
		mcp.WithArgument("path",
			mcp.ArgumentDescription("Vault-relative path to the note (e.g. \"Notes/my-note.md\")"),
			mcp.RequiredArgument(),
		),
	)
	s.AddPrompt(prompt, findRelatedHandler(deps))
}

func findRelatedHandler(deps Deps) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		if path == "" {
			return errorPrompt("find_related requires a 'path' argument"), nil
		}

		note, err := deps.Vault.ReadNote(ctx, path)
		if err != nil {
			return errorPrompt(fmt.Sprintf("could not read note %q: %v", path, err)), nil
		}

		tags, err := deps.Vault.ListTags(ctx, path)
		if err != nil {
			slog.Debug("find_related: ListTags failed", "path", path, "err", err)
		}
		backlinks, err := deps.Vault.GetBacklinks(ctx, path)
		if err != nil {
			slog.Debug("find_related: GetBacklinks failed", "path", path, "err", err)
		}
		outgoing := vault.ExtractLinks(note.Content)

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Note: %s\n\n", note.Path))
		sb.WriteString(fmt.Sprintf("Content:\n---\n%s\n---\n\n", note.Content))

		if len(tags) > 0 {
			sb.WriteString(fmt.Sprintf("Tags: %s\n\n", strings.Join(tags, ", ")))
		}
		if len(outgoing) > 0 {
			sb.WriteString(fmt.Sprintf("Outgoing links: %s\n\n", strings.Join(outgoing, ", ")))
		}
		if len(backlinks) > 0 {
			sb.WriteString("Notes that link here:\n")
			for _, bl := range backlinks {
				sb.WriteString(fmt.Sprintf("  - %s (line %d): %s\n", bl.Path, bl.Line, bl.Snippet))
			}
			sb.WriteString("\n")
		}

		sb.WriteString(`Based on the note content, tags, and existing links above, please suggest 5–10 related notes or topics worth linking. Group your suggestions by relationship type:
- Tag siblings: notes sharing the same tags
- Citations/references: notes this note should cite or be cited by
- Topical neighbors: notes covering closely related concepts
- Bidirectional opportunities: notes that link here but aren't linked back

For each suggestion, explain why the link would be valuable.`)

		return singleUserPrompt("Find related notes worth linking", sb.String()), nil
	}
}
