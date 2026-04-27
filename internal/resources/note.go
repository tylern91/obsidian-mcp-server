package resources

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerNoteResource(s *server.MCPServer, deps Deps) {
	tmpl := mcp.NewResourceTemplate(
		"obsidian://note/{path}",
		"Note content",
		mcp.WithTemplateDescription("Read any note in the vault by its vault-relative path. Returns the raw markdown including frontmatter."),
		mcp.WithTemplateMIMEType("text/markdown"),
	)
	s.AddResourceTemplate(tmpl, noteResourceHandler(deps))
}

func noteResourceHandler(deps Deps) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		uri := req.Params.URI
		notePath := pathFromURI(uri, "obsidian://note/")
		if notePath == "" {
			return resourceError(uri, fmt.Sprintf("cannot parse note path from URI %q", uri)), nil
		}

		note, err := deps.Vault.ReadNote(ctx, notePath)
		if err != nil {
			return resourceError(uri, fmt.Sprintf("note not found: %v", err)), nil
		}

		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/markdown",
			Text:     note.Content,
		}}, nil
	}
}
