package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// VaultService defines the vault operations that tool handlers depend on.
// Satisfied by *vault.Service; enables mock-based unit testing.
type VaultService interface {
	ReadNote(ctx context.Context, path string) (*vault.Note, error)
	WriteNote(ctx context.Context, path, content string, mode vault.WriteMode) error
	ListDirectory(ctx context.Context, path string) ([]vault.DirEntry, error)

	GetFrontmatter(ctx context.Context, path string) (fm map[string]any, body string, err error)
	UpdateFrontmatter(ctx context.Context, path string, updates map[string]any, removeKeys []string) error

	ListTags(ctx context.Context, path string) ([]string, error)
	AddTag(ctx context.Context, path, tag, location string) error
	RemoveTag(ctx context.Context, path, tag string) error
	AggregateTags(ctx context.Context) (map[string]int, error)

	GetBacklinks(ctx context.Context, targetPath string) ([]vault.Backlink, error)

	PatchNote(ctx context.Context, path string, p vault.PatchOp) error
	DeleteNote(ctx context.Context, path, confirm string) error
	MoveNote(ctx context.Context, src, dst, confirm string) error

	StatNote(ctx context.Context, path string) (*vault.NoteInfo, error)
}

// SearchService defines the search operations that tool handlers depend on.
// Satisfied by *search.Service; enables mock-based unit testing.
type SearchService interface {
	SearchBM25(ctx context.Context, opts search.BM25Options) ([]search.BM25Result, error)
	SearchRegex(ctx context.Context, opts search.RegexOptions) ([]search.RegexResult, error)
}

// Deps holds the dependencies injected into all tool handlers.
type Deps struct {
	Vault       VaultService
	Search      SearchService
	PrettyPrint bool // global default for JSON formatting
	MaxBatch    int  // maximum number of files per batch operation
	MaxResults  int  // maximum number of search results
}

// RegisterAll registers all MCP tools with the server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	registerReadNote(s, deps)
	registerWriteNote(s, deps)
	registerListDirectory(s, deps)
	registerGetFrontmatter(s, deps)
	registerUpdateFrontmatter(s, deps)
	registerManageTags(s, deps)
	registerListAllTags(s, deps)
	registerGetBacklinks(s, deps)
	registerPatchNote(s, deps)
	registerDeleteNote(s, deps)
	registerMoveNote(s, deps)
	registerSearchNotes(s, deps)
	registerSearchRegex(s, deps)
	registerReadMultipleNotes(s, deps)
	registerGetNotesInfo(s, deps)
}
