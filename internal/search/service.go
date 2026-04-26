package search

import (
	"context"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// VaultIterator is the narrow interface search.Service depends on.
// It is satisfied by *vault.Service.
//
// Design note: vault.SplitFrontmatter is a package-level function.
// The BM25 implementation imports vault directly to call it rather than
// adding a method to VaultIterator.
type VaultIterator interface {
	// WalkNotes calls fn for each allowed note in the vault.
	// fn receives the relative path (forward slashes) and the absolute path.
	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error

	// Root returns the absolute path to the vault root directory.
	Root() string
}

// Compile-time check that *vault.Service satisfies VaultIterator.
var _ VaultIterator = (*vault.Service)(nil)

// Service provides full-text search over a vault.
type Service struct {
	vault VaultIterator
}

// New creates a new search Service backed by the given vault.
func New(vault VaultIterator) *Service {
	return &Service{vault: vault}
}
