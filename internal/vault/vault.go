package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Note represents a read note from the vault.
type Note struct {
	Path    string    // relative path from vault root (e.g. "Notes/simple.md")
	AbsPath string    // absolute filesystem path (internal use)
	Content string    // raw file content including any frontmatter
	Size    int64     // file size in bytes
	ModTime time.Time // last modification time
}

// WriteMode controls how WriteNote behaves.
type WriteMode string

const (
	WriteModeOverwrite WriteMode = "overwrite" // replace entire content
	WriteModeAppend    WriteMode = "append"    // append to end
	WriteModePrepend   WriteMode = "prepend"   // prepend to start
)

// DirEntry represents a file or directory in the vault.
type DirEntry struct {
	Name    string // filename (not full path)
	Path    string // relative path from vault root
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// Service provides vault operations with path security.
type Service struct {
	root   string // symlink-resolved absolute path to vault root
	filter *PathFilter
	mu     sync.Mutex // protects concurrent append/prepend read-modify-write
}

// New creates a new vault Service.
// root must be an absolute path to an existing directory.
// Symlinks in the root path are resolved so that all subsequent path
// comparisons are consistent (important on macOS where /tmp -> /private/tmp).
func New(root string, filter *PathFilter) *Service {
	// Best-effort symlink resolution: if it fails, fall back to the original.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return &Service{
		root:   root,
		filter: filter,
	}
}

// isUnderRoot reports whether absPath is the root or a path under it.
func (s *Service) isUnderRoot(absPath string) bool {
	if absPath == s.root {
		return true
	}
	return strings.HasPrefix(absPath, s.root+string(filepath.Separator))
}

// ResolvePath returns the absolute path for a relative vault path.
// It applies security checks in order: lexical, filter, existence, symlink.
func (s *Service) ResolvePath(relativePath string) (string, error) {
	// Step 1: Lexical check.
	cleaned := filepath.Clean(relativePath)

	// Reject absolute paths.
	if filepath.IsAbs(cleaned) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathTraversal}
	}

	// Reject if cleaned path starts with ".." component.
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathTraversal}
	}

	absPath := filepath.Join(s.root, cleaned)

	// Verify the joined path is still under root (prefix check).
	if !s.isUnderRoot(absPath) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathTraversal}
	}

	// Step 2: Path filter check.
	if s.filter != nil && s.filter.IsIgnored(relativePath) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathRestricted}
	}

	// Extension check only for paths that have an extension (files, not dirs).
	if s.filter != nil && filepath.Ext(cleaned) != "" {
		if !s.filter.IsAllowedExtension(cleaned) {
			return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathRestricted}
		}
	}

	// Step 3: Existence check with case-insensitive fallback.
	finalAbs, err := s.existenceCheck(relativePath, absPath)
	if err != nil {
		return "", err
	}

	// Step 4: Symlink check.
	resolved, err := filepath.EvalSymlinks(finalAbs)
	if err != nil {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: err}
	}

	// Verify symlink target is still under root.
	if !s.isUnderRoot(resolved) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrSymlinkEscape}
	}

	return resolved, nil
}

// existenceCheck tries os.Stat and falls back to case-insensitive matching.
func (s *Service) existenceCheck(relativePath, absPath string) (string, error) {
	_, err := os.Stat(absPath)
	if err == nil {
		return absPath, nil
	}

	if !os.IsNotExist(err) {
		return "", &PathError{Op: "resolve", Path: relativePath, Err: err}
	}

	// Case-insensitive fallback: search parent directory.
	parentDir := filepath.Dir(absPath)
	targetName := filepath.Base(absPath)

	entries, readErr := os.ReadDir(parentDir)
	if readErr != nil {
		// Parent doesn't exist either — report original not found.
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrNotFound}
	}

	var matches []string
	targetLower := strings.ToLower(targetName)
	for _, entry := range entries {
		if strings.ToLower(entry.Name()) == targetLower {
			matches = append(matches, filepath.Join(parentDir, entry.Name()))
		}
	}

	switch len(matches) {
	case 0:
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrNotFound}
	case 1:
		return matches[0], nil
	default:
		return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrAmbiguousPath}
	}
}

// ReadNote reads a note at the given relative path and returns its content and metadata.
func (s *Service) ReadNote(ctx context.Context, path string) (*Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	absPath, err := s.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	return &Note{
		Path:    path,
		AbsPath: absPath,
		Content: string(data),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

// WriteNote writes content to a note at the given relative path.
// The write mode controls whether the content is overwritten, appended, or prepended.
// WriteNote does NOT apply extension filtering — any file that passes the ignore
// filter may be written.
func (s *Service) WriteNote(ctx context.Context, path, content string, mode WriteMode) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "write", Path: path, Err: err}
	}

	// Lexical check (same as ResolvePath step 1, but without existence or symlink checks).
	cleaned := filepath.Clean(path)

	if filepath.IsAbs(cleaned) {
		return &PathError{Op: "write", Path: path, Err: ErrPathTraversal}
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return &PathError{Op: "write", Path: path, Err: ErrPathTraversal}
	}

	absPath := filepath.Join(s.root, cleaned)
	if !s.isUnderRoot(absPath) {
		return &PathError{Op: "write", Path: path, Err: ErrPathTraversal}
	}

	// Filter check (ignore only — no extension check for writes).
	if s.filter != nil && s.filter.IsIgnored(path) {
		return &PathError{Op: "write", Path: path, Err: ErrPathRestricted}
	}

	parentDir := filepath.Dir(absPath)

	// Symlink escape check on parent directory (parent may not exist yet for new files).
	if parentInfo, statErr := os.Stat(parentDir); statErr == nil && parentInfo.IsDir() {
		resolvedParent, evalErr := filepath.EvalSymlinks(parentDir)
		if evalErr != nil {
			return &PathError{Op: "write", Path: path, Err: evalErr}
		}
		if !s.isUnderRoot(resolvedParent) {
			return &PathError{Op: "write", Path: path, Err: ErrSymlinkEscape}
		}
	}

	switch mode {
	case WriteModeOverwrite:
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}

	case WriteModeAppend, WriteModePrepend:
		s.mu.Lock()
		defer s.mu.Unlock()

		existing := readExistingOrEmpty(absPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}

		var combined string
		if mode == WriteModeAppend {
			combined = existing + content
		} else {
			combined = content + existing
		}

		if err := os.WriteFile(absPath, []byte(combined), 0644); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}

	default:
		return fmt.Errorf("unknown write mode: %q", mode)
	}

	return nil
}

// readExistingOrEmpty reads file content, returning empty string if file doesn't exist.
func readExistingOrEmpty(absPath string) string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// ListDirectory lists the entries in a vault directory.
// If path is empty, it lists the vault root.
func (s *Service) ListDirectory(ctx context.Context, path string) ([]DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}

	var absPath string

	if path == "" {
		absPath = s.root
	} else {
		// Lexical check.
		cleaned := filepath.Clean(path)

		if filepath.IsAbs(cleaned) {
			return nil, &PathError{Op: "list", Path: path, Err: ErrPathTraversal}
		}
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return nil, &PathError{Op: "list", Path: path, Err: ErrPathTraversal}
		}

		absPath = filepath.Join(s.root, cleaned)
		if !s.isUnderRoot(absPath) {
			return nil, &PathError{Op: "list", Path: path, Err: ErrPathTraversal}
		}

		// Filter check.
		if s.filter != nil && s.filter.IsIgnored(path) {
			return nil, &PathError{Op: "list", Path: path, Err: ErrPathRestricted}
		}

		// Existence check for the directory itself.
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return nil, &PathError{Op: "list", Path: path, Err: ErrNotFound}
			}
			return nil, &PathError{Op: "list", Path: path, Err: err}
		}
	}

	rawEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}

	var results []DirEntry
	for _, entry := range rawEntries {
		name := entry.Name()

		// Skip ignored entries.
		if s.filter != nil && s.filter.IsIgnored(name) {
			continue
		}

		var relPath string
		if path == "" {
			relPath = name
		} else {
			relPath = filepath.Join(path, name)
		}

		info, err := entry.Info()
		if err != nil {
			// Skip entries we can't stat.
			continue
		}

		results = append(results, DirEntry{
			Name:    name,
			Path:    relPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return results, nil
}
