package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
)

func registerSearchNotes(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("search_notes",
		mcp.WithDescription("Search vault notes using BM25 ranked full-text search. Returns results sorted by relevance score."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query. Multi-term queries use OR logic; the full phrase contributes a bonus score."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default 20)"),
		),
		mcp.WithNumber("maxMatchesPerFile",
			mcp.Description("Maximum match snippets per result (default 3)"),
		),
		mcp.WithBoolean("caseSensitive",
			mcp.Description("Case-sensitive matching (default false)"),
		),
		mcp.WithBoolean("searchContent",
			mcp.Description("Include note body in scoring (default true)"),
		),
		mcp.WithBoolean("searchFrontmatter",
			mcp.Description("Include frontmatter values in scoring (default true)"),
		),
		mcp.WithString("pathScope",
			mcp.Description("Glob pattern to restrict search scope (e.g. 'Daily Notes/*')"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format JSON with indentation"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, searchNotesHandler(deps))
}

func searchNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)

		opts := search.BM25Options{
			Query:             query,
			Limit:             req.GetInt("limit", 0),
			MaxMatchesPerFile: req.GetInt("maxMatchesPerFile", 0),
			CaseSensitive:     req.GetBool("caseSensitive", false),
			SearchContent:     req.GetBool("searchContent", true),
			SearchFrontmatter: req.GetBool("searchFrontmatter", true),
			PathScope:         req.GetString("pathScope", ""),
		}

		results, err := deps.Search.SearchBM25(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if results == nil {
			results = []search.BM25Result{}
		}

		type searchNotesResponse struct {
			Query   string            `json:"query"`
			Results []search.BM25Result `json:"results"`
			Total   int               `json:"total"`
		}
		out, err := response.FormatJSON(searchNotesResponse{
			Query:   query,
			Results: results,
			Total:   len(results),
		}, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}

func registerSearchRegex(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("search_regex",
		mcp.WithDescription("Search vault notes using a regex or glob pattern."),
		mcp.WithString("pattern",
			mcp.Required(),
			mcp.Description("RE2 regular expression or filepath glob pattern to search for."),
		),
		mcp.WithBoolean("isGlob",
			mcp.Description("Treat pattern as a filepath glob (default false)"),
		),
		mcp.WithString("scope",
			mcp.Description("Search scope: 'path', 'content', or 'both' (default 'content')"),
			mcp.Enum("path", "content", "both"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results (default 20)"),
		),
		mcp.WithNumber("maxMatchesPerFile",
			mcp.Description("Maximum match snippets per file (default 5)"),
		),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format JSON with indentation"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, searchRegexHandler(deps))
}

func searchRegexHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pattern, err := req.RequireString("pattern")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)
		scope := req.GetString("scope", "content")

		opts := search.RegexOptions{
			Pattern:           pattern,
			IsGlob:            req.GetBool("isGlob", false),
			Scope:             scope,
			Limit:             req.GetInt("limit", 0),
			MaxMatchesPerFile: req.GetInt("maxMatchesPerFile", 0),
		}

		results, err := deps.Search.SearchRegex(ctx, opts)
		if err != nil {
			// Return as tool error content, not a Go error, so callers see a
			// structured response (e.g. invalid regex pattern).
			return mcp.NewToolResultError(err.Error()), nil
		}
		if results == nil {
			results = []search.RegexResult{}
		}

		type searchRegexResponse struct {
			Pattern string              `json:"pattern"`
			Scope   string              `json:"scope"`
			Results []search.RegexResult `json:"results"`
			Total   int                 `json:"total"`
		}
		out, err := response.FormatJSON(searchRegexResponse{
			Pattern: pattern,
			Scope:   scope,
			Results: results,
			Total:   len(results),
		}, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}
