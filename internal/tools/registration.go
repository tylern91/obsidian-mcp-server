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
	WriteNote(ctx context.Context, path, content string, mode vault.WriteMode, opts ...vault.WriteOpt) error
	ListDirectory(ctx context.Context, path string) ([]vault.DirEntry, error)

	GetFrontmatter(ctx context.Context, path string) (fm map[string]any, body string, err error)
	UpdateFrontmatter(ctx context.Context, path string, updates map[string]any, removeKeys []string, opts ...vault.WriteOpt) error

	ListTags(ctx context.Context, path string) ([]string, error)
	AddTag(ctx context.Context, path, tag, location string, opts ...vault.WriteOpt) error
	RemoveTag(ctx context.Context, path, tag string, opts ...vault.WriteOpt) error
	AggregateTags(ctx context.Context) (map[string]int, error)

	GetBacklinks(ctx context.Context, targetPath string) ([]vault.Backlink, error)

	PatchNote(ctx context.Context, path string, p vault.PatchOp, opts ...vault.WriteOpt) error
	DeleteNote(ctx context.Context, path, confirm string, permanent bool, opts ...vault.WriteOpt) error
	MoveNote(ctx context.Context, src, dst, confirm string, updateLinks, dryRun bool, opts ...vault.WriteOpt) (*vault.MoveResult, error)
	RenameTag(ctx context.Context, oldTag, newTag string) ([]vault.TagRename, error)
	ReplaceInNote(ctx context.Context, path, pattern, replacement string, isRegex bool, maxOccurrences int) (*vault.ReplaceResult, error)

	StatNote(ctx context.Context, path string) (*vault.NoteInfo, error)

	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error
	VaultStats(ctx context.Context, opts vault.VaultStatsOpts) (*vault.VaultStats, error)
	Root() string

	GetNoteOutline(ctx context.Context, path string) ([]vault.HeadingInfo, error)
	ReadNoteLines(ctx context.Context, path string, startLine, lineCount int) (*vault.NoteLines, error)
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
	PrettyPrint bool   // global default for JSON formatting
	MaxBatch    int    // maximum number of files per batch operation
	MaxResults  int    // maximum number of search results
	ReadOnly    bool   // when true, mutating tools are neither registered nor callable
	VaultName   string // vault name used to build obsidian:// deep links
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
		getNoteOutlineSpec(deps),
		readNoteLinesSpec(deps),
		renameTagSpec(deps),
		replaceInNoteSpec(deps),
	}
}

// RegisterAll registers all MCP tools with the server. In read-only mode
// (deps.ReadOnly), mutating tools are omitted entirely — they never appear
// in tools/list, so a client (or the model driving it) can't even attempt
// one. See readOnlyGuard for the second layer of that defense.
func RegisterAll(s *server.MCPServer, deps Deps) {
	for _, spec := range allSpecs(deps) {
		handler := spec.Handler
		if spec.Mutating {
			handler = readOnlyGuard(deps.ReadOnly, handler)
			if deps.ReadOnly {
				continue
			}
		}
		s.AddTool(spec.Tool, handler)
	}
}

// readOnlyGuard wraps a mutating tool's handler so it refuses to run when
// the server is in read-only mode. RegisterAll already omits mutating tools
// from registration in that mode; this wrap exists so the rejection is
// derived from toolSpec.Mutating in this one place rather than hand-written
// per handler, and holds even if a future change to RegisterAll's loop
// stops omitting the tool.
func readOnlyGuard(readOnly bool, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	if !readOnly {
		return handler
	}
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("server is running in --read-only mode"), nil
	}
}
