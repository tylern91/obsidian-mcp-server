package prompts

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// VaultService defines the vault operations used by prompt handlers.
type VaultService interface {
	ReadNote(ctx context.Context, path string) (*vault.Note, error)
	GetBacklinks(ctx context.Context, targetPath string) ([]vault.Backlink, error)
	ListTags(ctx context.Context, path string) ([]string, error)
	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error
	Root() string
}

// PeriodicService defines the periodic note operations used by prompt handlers.
type PeriodicService interface {
	Resolve(granularity string, offset int) (string, error)
	RecentDates(granularity string, count int) ([]time.Time, error)
}

// Deps holds dependencies injected into prompt handlers.
type Deps struct {
	Vault    VaultService
	Periodic PeriodicService
}

// RegisterAll registers all MCP prompts with the server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	registerSummarizeNote(s, deps)
	registerDailyNoteReview(s, deps)
	registerWeeklyReview(s, deps)
	registerFindRelated(s, deps)
	registerVaultHealthCheck(s, deps)
}
