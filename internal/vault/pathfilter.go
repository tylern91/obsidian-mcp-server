package vault

import (
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// PathFilter determines whether a vault path should be ignored or is
// allowed based on configurable patterns and file extensions.
type PathFilter struct {
	ignorePatterns []ignorePattern
	allowedExts    []string // e.g. [".md", ".markdown", ".txt", ".canvas"]
}

// ignorePattern is one parsed ignore entry. An entry without a slash matches
// any single path component; an entry with a slash is a root-relative path
// prefix held as its components.
type ignorePattern struct {
	component string   // any-component pattern (NFC); empty when parts is set
	parts     []string // root-relative prefix components (NFC)
}

// internalStateDir is the top-level directory obsidian-mcp-server uses for
// its own bookkeeping (currently: delete_note's trash). It is always
// excluded from path resolution, directory listings, and walks —
// independent of the configured ignore patterns and even when no
// PathFilter is configured — so a user overriding --ignore can never
// accidentally resurface trashed notes as live vault content.
const internalStateDir = ".obsidian-mcp"

// IsInternalState reports whether path's first component is the server's
// internal state directory. Unlike IsIgnored, this check is unconditional:
// call sites must apply it regardless of whether a *PathFilter is present.
func IsInternalState(path string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(path), "/")
	return strings.EqualFold(first, internalStateDir)
}

// NewPathFilter creates a PathFilter with the given ignore patterns and
// allowed extensions. Either slice may be nil or empty. Ignore patterns are
// stored NFC-normalized; macOS file systems return NFD names, so IsIgnored
// normalizes candidate paths the same way.
//
// An entry containing "/" is a root-relative path prefix: leading and trailing
// slashes are trimmed and the remaining components must match the start of the
// candidate path on component boundaries. An entry without "/" matches any
// single component at any depth.
func NewPathFilter(ignorePatterns, allowedExts []string) *PathFilter {
	var parsed []ignorePattern
	for _, p := range ignorePatterns {
		p = norm.NFC.String(strings.TrimSpace(p))
		if strings.Contains(p, "/") {
			var parts []string
			for _, c := range strings.Split(strings.Trim(p, "/"), "/") {
				if c != "" {
					parts = append(parts, c)
				}
			}
			if len(parts) == 0 {
				continue
			}
			parsed = append(parsed, ignorePattern{parts: parts})
			continue
		}
		parsed = append(parsed, ignorePattern{component: p})
	}
	return &PathFilter{
		ignorePatterns: parsed,
		allowedExts:    allowedExts,
	}
}

// ValidateIgnorePattern reports why an ignore entry is unusable: empty, ".",
// "..", or containing a ".." component. A nil return means the entry is valid.
func ValidateIgnorePattern(entry string) error {
	e := strings.TrimSpace(entry)
	if e == "" {
		return errors.New("ignore entry is empty")
	}
	trimmed := strings.Trim(e, "/")
	if trimmed == "" || trimmed == "." {
		return fmt.Errorf("ignore entry %q is not a usable pattern", entry)
	}
	for _, c := range strings.Split(trimmed, "/") {
		if c == ".." {
			return fmt.Errorf("ignore entry %q must not contain a \"..\" component", entry)
		}
		if c == "." {
			return fmt.Errorf("ignore entry %q must not contain a \".\" component", entry)
		}
	}
	return nil
}

// IsIgnored reports whether path matches one of the configured ignore
// patterns. Matching is case-insensitive (the vault may live on a
// case-insensitive filesystem), NFC-normalized, and always on whole-component
// boundaries: a partial prefix like ".git-backup" does not match ".git", and
// "Journal/PrivateStuff" does not match the path entry "Journal/Private".
// Path entries (containing "/") are anchored at the vault root.
func (f *PathFilter) IsIgnored(path string) bool {
	if len(f.ignorePatterns) == 0 {
		return false
	}

	// Lexically clean first so no caller can bypass a root-anchored entry with
	// an unclean path such as "Other/../Journal/Private/x.md". Backslashes are
	// treated as separators too: over-matching is the safe direction here.
	slashed := strings.ReplaceAll(filepath.ToSlash(path), `\`, "/")
	normalized := norm.NFC.String(pathpkg.Clean(slashed))
	var components []string
	for _, c := range strings.Split(normalized, "/") {
		if c != "" && c != "." {
			components = append(components, c)
		}
	}

	for _, pattern := range f.ignorePatterns {
		if pattern.parts != nil {
			if len(components) >= len(pattern.parts) && prefixEqualFold(components, pattern.parts) {
				return true
			}
			continue
		}
		for _, component := range components {
			if strings.EqualFold(component, pattern.component) {
				return true
			}
		}
	}
	return false
}

func prefixEqualFold(components, parts []string) bool {
	for i, p := range parts {
		if !strings.EqualFold(components[i], p) {
			return false
		}
	}
	return true
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

// Stem returns the file's name without its extension.
//
// Example: Stem("Notes/Daily/2025-01-15.md") → "2025-01-15"
func Stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// StemLower is Stem(path) lowercased — useful for case-insensitive lookups
// such as wikilink resolution where Obsidian matches case-insensitively.
func StemLower(path string) string {
	return strings.ToLower(Stem(path))
}
