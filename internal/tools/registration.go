package tools

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// Deps holds the dependencies injected into all tool handlers.
type Deps struct {
	Vault       *vault.Service
	PrettyPrint bool // global default for JSON formatting
}

// RegisterAll registers all Phase 1 tools with the MCP server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	registerReadNote(s, deps)
	registerWriteNote(s, deps)
	registerListDirectory(s, deps)
}
