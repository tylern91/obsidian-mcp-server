package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func registerManageTags(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("manage_tags",
		mcp.WithDescription("Add or remove a tag on a note. Use location to control where new tags are placed."),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the note relative to the vault root"),
		),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Operation to perform: \"add\" or \"remove\""),
			mcp.Enum("add", "remove"),
		),
		mcp.WithString("tag",
			mcp.Required(),
			mcp.Description("Tag to add or remove (without the # prefix)"),
		),
		mcp.WithString("location",
			mcp.Description("Where to add new tags: \"frontmatter\" (default) or \"inline\""),
			mcp.Enum("frontmatter", "inline"),
			mcp.DefaultString("frontmatter"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	)
	s.AddTool(tool, manageTagsHandler(deps))
}

func manageTagsHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		action, err := req.RequireString("action")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		tag, err := req.RequireString("tag")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		location := req.GetString("location", "frontmatter")

		switch action {
		case "add":
			if err := deps.Vault.AddTag(ctx, path, tag, location); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		case "remove":
			if err := deps.Vault.RemoveTag(ctx, path, tag); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		default:
			return mcp.NewToolResultError("unknown action: " + action), nil
		}

		type tagResponse struct {
			Success  bool   `json:"success"`
			Path     string `json:"path"`
			Action   string `json:"action"`
			Tag      string `json:"tag"`
			Location string `json:"location,omitempty"`
		}
		resp := tagResponse{Success: true, Path: path, Action: action, Tag: tag}
		if action == "add" {
			resp.Location = location
		}
		result, err := response.FormatJSON(resp, deps.PrettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

func registerListAllTags(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("list_all_tags",
		mcp.WithDescription("Aggregate all tags across the entire vault with usage counts."),
		mcp.WithBoolean("prettyPrint",
			mcp.Description("Format the JSON response with indentation (default: false)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, listAllTagsHandler(deps))
}

func listAllTagsHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prettyPrint := req.GetBool("prettyPrint", deps.PrettyPrint)

		tagCounts, err := deps.Vault.AggregateTags(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		top := vault.TopTagsByCount(tagCounts, len(tagCounts))

		type allTagsResponse struct {
			Tags  []vault.TagCount `json:"tags"`
			Total int              `json:"total"`
		}
		result, err := response.FormatJSON(allTagsResponse{Tags: top, Total: len(top)}, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
