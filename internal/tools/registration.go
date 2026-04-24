package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// VaultService defines the vault operations that tool handlers depend on.
// Satisfied by *vault.Service; enables mock-based unit testing.
type VaultService interface {
	ReadNote(ctx context.Context, path string) (*vault.Note, error)
	WriteNote(ctx context.Context, path, content string, mode vault.WriteMode) error
	ListDirectory(ctx context.Context, path string) ([]vault.DirEntry, error)
}

// Deps holds the dependencies injected into all tool handlers.
type Deps struct {
	Vault       VaultService
	PrettyPrint bool // global default for JSON formatting
}

// RegisterAll registers all Phase 1 tools with the MCP server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	registerReadNote(s, deps)
	registerWriteNote(s, deps)
	registerListDirectory(s, deps)
}
