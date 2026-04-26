package vault

import (
	"context"
	"io/fs"
	"path/filepath"
)

// WalkNotes calls fn for each allowed note file in the vault, in filesystem
// order. Directories matching the vault's ignore list are skipped. fn receives
// the path relative to the vault root (forward slashes) and the absolute path.
// Returning filepath.SkipAll from fn aborts the walk early with no error.
//
// The method is read-only and does not hold the service mutex.
func (s *Service) WalkNotes(ctx context.Context, fn func(rel, abs string) error) error {
	return filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries silently
		}

		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return nil
		}

		if d.IsDir() {
			if rel != "." && s.filter != nil && s.filter.IsIgnored(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		if s.filter != nil {
			if s.filter.IsIgnored(rel) {
				return nil
			}
			if !s.filter.IsAllowedExtension(filepath.Ext(path)) {
				return nil
			}
		}

		return fn(filepath.ToSlash(rel), path)
	})
}
