package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerGetPeriodicNote and registerGetRecentPeriodicNotes are stubs.
// They will be fully implemented in the get_periodic_note slice.

func registerGetPeriodicNote(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_periodic_note",
		mcp.WithDescription("Get the path for a periodic note (daily, weekly, monthly, quarterly, yearly)"),
	)
	s.AddTool(tool, getPeriodicNoteHandler(deps))
}

func getPeriodicNoteHandler(deps Deps) server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("not implemented"), nil
	}
}

func registerGetRecentPeriodicNotes(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("get_recent_periodic_notes",
		mcp.WithDescription("List recent periodic notes for a given granularity"),
	)
	s.AddTool(tool, getRecentPeriodicNotesHandler(deps))
}

func getRecentPeriodicNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("not implemented"), nil
	}
}
