package resources

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerPeriodicResource(s *server.MCPServer, deps Deps) {
	tmpl := mcp.NewResourceTemplate(
		"obsidian://periodic/{granularity}",
		"Periodic note",
		mcp.WithTemplateDescription("Read the current periodic note by granularity: daily, weekly, monthly, quarterly, or yearly."),
		mcp.WithTemplateMIMEType("text/markdown"),
	)
	s.AddResourceTemplate(tmpl, periodicResourceHandler(deps))
}

func periodicResourceHandler(deps Deps) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		uri := req.Params.URI
		granularity := pathFromURI(uri, "obsidian://periodic/")
		if granularity == "" {
			return resourceError(uri, fmt.Sprintf("cannot parse granularity from URI %q", uri)), nil
		}

		notePath, err := deps.Periodic.Resolve(granularity, 0)
		if err != nil {
			return resourceError(uri, fmt.Sprintf("could not resolve %s periodic note: %v", granularity, err)), nil
		}

		note, err := deps.Vault.ReadNote(ctx, notePath)
		if err != nil {
			// Note doesn't exist yet — return an explanatory empty content rather than an error.
			return []mcp.ResourceContents{mcp.TextResourceContents{
				URI:      uri,
				MIMEType: "text/markdown",
				Text:     fmt.Sprintf("<!-- %s periodic note at %q does not exist yet -->", granularity, notePath),
			}}, nil
		}

		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/markdown",
			Text:     note.Content,
		}}, nil
	}
}
