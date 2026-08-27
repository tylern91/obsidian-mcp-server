package resources

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func registerBacklinksResource(s *server.MCPServer, deps Deps) {
	tmpl := mcp.NewResourceTemplate(
		"obsidian://backlinks/{path}",
		"Note backlinks",
		mcp.WithTemplateDescription("All notes that link to the specified note, with line numbers and snippets."),
		mcp.WithTemplateMIMEType("application/json"),
	)
	s.AddResourceTemplate(tmpl, backlinksResourceHandler(deps))
}

func backlinksResourceHandler(deps Deps) server.ResourceTemplateHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		uri := req.Params.URI
		notePath := pathFromURI(uri, "obsidian://backlinks/")
		if notePath == "" {
			return response.ErrorResourceContents(uri, fmt.Sprintf("cannot parse note path from URI %q", uri)), nil
		}

		backlinks, err := deps.Vault.GetBacklinks(ctx, notePath)
		if err != nil {
			return response.ErrorResourceContents(uri, fmt.Sprintf("backlink lookup failed: %v", err)), nil
		}

		type entry struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		}
		entries := make([]entry, 0, len(backlinks))
		for _, bl := range backlinks {
			entries = append(entries, entry{Path: bl.Path, Line: bl.Line, Snippet: bl.Snippet})
		}

		type result struct {
			Target    string  `json:"target"`
			Backlinks []entry `json:"backlinks"`
			Total     int     `json:"total"`
		}
		return response.JSONResourceContents(uri, "application/json", result{
			Target:    notePath,
			Backlinks: entries,
			Total:     len(entries),
		}, deps.PrettyPrint), nil
	}
}
