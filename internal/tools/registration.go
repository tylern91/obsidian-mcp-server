package tools

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
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

	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error
	VaultStats(ctx context.Context, opts vault.VaultStatsOpts) (*vault.VaultStats, error)
	Root() string
}

// SearchService defines the search operations that tool handlers depend on.
// Satisfied by *search.Service; enables mock-based unit testing.
type SearchService interface {
	SearchBM25(ctx context.Context, opts search.BM25Options) ([]search.BM25Result, error)
	SearchRegex(ctx context.Context, opts search.RegexOptions) ([]search.RegexResult, error)
}

// PeriodicService defines the periodic note operations that tool handlers depend on.
// Satisfied by *periodic.Service; enables mock-based unit testing.
type PeriodicService interface {
	Resolve(granularity string, offset int) (string, error)
	RecentDates(granularity string, count int) ([]time.Time, error)
}

// Deps holds the dependencies injected into all tool handlers.
type Deps struct {
	Vault       VaultService
	Search      SearchService
	Periodic    PeriodicService
	PrettyPrint bool // global default for JSON formatting
	MaxBatch    int  // maximum number of files per batch operation
	MaxResults  int  // maximum number of search results
}

// toolSpec bundles an MCP tool definition with its handler and whether it
// mutates the vault. Mutating is derived from the tool's own ReadOnlyHint so
// it cannot drift from the annotation the client sees.
type toolSpec struct {
	Tool     mcp.Tool
	Handler  server.ToolHandlerFunc
	Mutating bool
}

// newToolSpec derives Mutating from tool's ReadOnlyHint annotation. A tool
// with no annotation is treated as mutating (fail safe).
func newToolSpec(tool mcp.Tool, handler server.ToolHandlerFunc) toolSpec {
	mutating := true
	if tool.Annotations.ReadOnlyHint != nil {
		mutating = !*tool.Annotations.ReadOnlyHint
	}
	return toolSpec{Tool: tool, Handler: handler, Mutating: mutating}
}

// allSpecs builds the full set of tool specs. Exported via export_test.go for
// coverage of registration wiring itself.
func allSpecs(deps Deps) []toolSpec {
	return []toolSpec{
		readNoteSpec(deps),
		writeNoteSpec(deps),
		listDirectorySpec(deps),
		getFrontmatterSpec(deps),
		updateFrontmatterSpec(deps),
		manageTagsSpec(deps),
		listAllTagsSpec(deps),
		getBacklinksSpec(deps),
		patchNoteSpec(deps),
		deleteNoteSpec(deps),
		moveNoteSpec(deps),
		searchNotesSpec(deps),
		searchRegexSpec(deps),
		readMultipleNotesSpec(deps),
		getNotesInfoSpec(deps),
		getVaultStatsSpec(deps),
		getRecentChangesSpec(deps),
		getPeriodicNoteSpec(deps),
		getRecentPeriodicNotesSpec(deps),
		auditNotesSpec(deps),
	}
}

// RegisterAll registers all MCP tools with the server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	for _, spec := range allSpecs(deps) {
		s.AddTool(spec.Tool, spec.Handler)
	}
}
