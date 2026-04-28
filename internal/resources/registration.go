package resources

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// VaultService defines the vault operations used by resource handlers.
type VaultService interface {
	ReadNote(ctx context.Context, path string) (*vault.Note, error)
	AggregateTags(ctx context.Context) (map[string]int, error)
	VaultStats(ctx context.Context, opts vault.VaultStatsOpts) (*vault.VaultStats, error)
	GetBacklinks(ctx context.Context, targetPath string) ([]vault.Backlink, error)
	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error
	Root() string
}

// PeriodicService defines the periodic note operations used by resource handlers.
type PeriodicService interface {
	Resolve(granularity string, offset int) (string, error)
	RecentDates(granularity string, count int) ([]time.Time, error)
}

// Deps holds dependencies injected into resource handlers.
type Deps struct {
	Vault       VaultService
	Periodic    PeriodicService
	PrettyPrint bool
}

// RegisterAll registers all MCP resources and resource templates with the server.
func RegisterAll(s *server.MCPServer, deps Deps) {
	registerVaultStats(s, deps)
	registerVaultTags(s, deps)
	registerNoteResource(s, deps)
	registerPeriodicResource(s, deps)
	registerBacklinksResource(s, deps)
}
