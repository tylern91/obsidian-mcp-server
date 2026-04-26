package search

import (
	"context"
)

// VaultIterator is the narrow interface search.Service depends on.
// It is satisfied by *vault.Service.
//
// Design note: vault.SplitFrontmatter is a package-level function, so it is
// not included here. The search package imports vault directly and calls
// vault.SplitFrontmatter where needed, avoiding an unnecessary wrapper method.
type VaultIterator interface {
	// WalkNotes calls fn for each allowed note in the vault.
	// fn receives the relative path (forward slashes) and the absolute path.
	WalkNotes(ctx context.Context, fn func(rel, abs string) error) error

	// Root returns the absolute path to the vault root directory.
	Root() string
}

// Service provides full-text search over a vault.
type Service struct {
	vault VaultIterator
}

// New creates a new search Service backed by the given vault.
func New(vault VaultIterator) *Service {
	return &Service{vault: vault}
}
