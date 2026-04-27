package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

		tags, _ := deps.Vault.ListTags(ctx, path)
		backlinks, _ := deps.Vault.GetBacklinks(ctx, path)
		outgoing := extractWikilinks(note.Content)

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

		return mcp.NewGetPromptResult(
			"Find related notes worth linking",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(sb.String())),
			},
		), nil
	}
}

// extractWikilinks extracts [[target]] link targets from note content.
func extractWikilinks(content string) []string {
	var links []string
	seen := make(map[string]bool)
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '[' && content[i+1] == '[' {
			end := strings.Index(content[i+2:], "]]")
			if end < 0 {
				continue
			}
			target := content[i+2 : i+2+end]
			if pipe := strings.Index(target, "|"); pipe >= 0 {
				target = target[:pipe]
			}
			target = strings.TrimSpace(target)
			if target != "" && !seen[target] {
				seen[target] = true
				links = append(links, target)
			}
			i += 2 + end + 1
		}
	}
	return links
}
