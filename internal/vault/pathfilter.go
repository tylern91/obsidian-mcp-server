package vault

import (
	"path/filepath"
	"strings"
)

// PathFilter determines whether a vault path should be ignored or is
// allowed based on configurable patterns and file extensions.
type PathFilter struct {
	ignorePatterns []string // e.g. [".obsidian", ".git", "node_modules"]
	allowedExts    []string // e.g. [".md", ".markdown", ".txt", ".canvas"]
}

// NewPathFilter creates a PathFilter with the given ignore patterns and
// allowed extensions. Either slice may be nil or empty.
func NewPathFilter(ignorePatterns, allowedExts []string) *PathFilter {
	return &PathFilter{
		ignorePatterns: ignorePatterns,
		allowedExts:    allowedExts,
	}
}

// IsIgnored reports whether any path component (directory or file name)
// exactly matches one of the configured ignore patterns.
// Matching is case-sensitive and requires an exact component match —
// a partial prefix like ".git-backup" does not match the pattern ".git".
func (f *PathFilter) IsIgnored(path string) bool {
	if len(f.ignorePatterns) == 0 {
		return false
	}

	// Normalize separators so we can split uniformly.
	normalized := filepath.ToSlash(path)
	components := strings.Split(normalized, "/")

	for _, component := range components {
		if component == "" {
			continue
		}
		for _, pattern := range f.ignorePatterns {
			if component == pattern {
				return true
			}
		}
	}
	return false
}

// IsAllowedExtension reports whether the file extension of path is in the
// allowed-extensions list. Matching is case-insensitive.
// If allowedExts is empty, all extensions are considered allowed.
func (f *PathFilter) IsAllowedExtension(path string) bool {
	if len(f.allowedExts) == 0 {
		return true
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}

	for _, allowed := range f.allowedExts {
		if strings.ToLower(allowed) == ext {
			return true
		}
	}
	return false
}
