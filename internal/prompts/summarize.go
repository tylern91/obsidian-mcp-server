package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSummarizeNote(s *server.MCPServer, deps Deps) {
	prompt := mcp.NewPrompt("summarize_note",
		mcp.WithPromptDescription("Summarize an Obsidian note: 3 key bullets, surface important entities, list open questions."),
		mcp.WithArgument("path",
			mcp.ArgumentDescription("Vault-relative path to the note (e.g. \"Notes/my-note.md\")"),
			mcp.RequiredArgument(),
		),
	)
	s.AddPrompt(prompt, summarizeNoteHandler(deps))
}

func summarizeNoteHandler(deps Deps) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		path := req.Params.Arguments["path"]
		if path == "" {
			return errorPrompt("summarize_note requires a 'path' argument"), nil
		}

		note, err := deps.Vault.ReadNote(ctx, path)
		if err != nil {
			return errorPrompt(fmt.Sprintf("could not read note %q: %v", path, err)), nil
		}

		text := fmt.Sprintf(`You are reviewing an Obsidian note. Please provide:
1. A 3-bullet summary of the key ideas
2. Important entities (people, projects, concepts) mentioned
3. Any open questions or unresolved items

Note path: %s

Note content:
---
%s
---`, note.Path, note.Content)

		return mcp.NewGetPromptResult(
			"Summarize an Obsidian note",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
			},
		), nil
	}
}
