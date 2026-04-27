package vault

import (
	"context"
	"fmt"
	"io"
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
	mu     sync.Mutex // protects concurrent file writes
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

// sanitizePath performs the shared lexical and filter validation (steps 1-2)
// common to ResolvePath, WriteNote, and ListDirectory.
// It rejects null bytes, absolute paths, ".." traversal, and ignored patterns.
func (s *Service) sanitizePath(op, relativePath string) (cleaned, absPath string, err error) {
	if strings.ContainsRune(relativePath, 0) {
		return "", "", &PathError{Op: op, Path: relativePath, Err: ErrPathTraversal}
	}

	cleaned = filepath.Clean(relativePath)

	if filepath.IsAbs(cleaned) {
		return "", "", &PathError{Op: op, Path: relativePath, Err: ErrPathTraversal}
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", &PathError{Op: op, Path: relativePath, Err: ErrPathTraversal}
	}

	absPath = filepath.Join(s.root, cleaned)

	if !s.isUnderRoot(absPath) {
		return "", "", &PathError{Op: op, Path: relativePath, Err: ErrPathTraversal}
	}

	if s.filter != nil && s.filter.IsIgnored(relativePath) {
		return "", "", &PathError{Op: op, Path: relativePath, Err: ErrPathRestricted}
	}

	return cleaned, absPath, nil
}

// resolveSymlink calls EvalSymlinks and verifies the target is under the vault root.
func (s *Service) resolveSymlink(op, relativePath, absPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", &PathError{Op: op, Path: relativePath, Err: err}
	}
	if !s.isUnderRoot(resolved) {
		return "", &PathError{Op: op, Path: relativePath, Err: ErrSymlinkEscape}
	}
	return resolved, nil
}

// ResolvePath returns the absolute path for a relative vault path.
// It applies security checks in order: lexical, filter, existence, symlink.
func (s *Service) ResolvePath(relativePath string) (string, error) {
	cleaned, absPath, err := s.sanitizePath("resolve", relativePath)
	if err != nil {
		return "", err
	}

	// Extension check only for paths that have an extension (files, not dirs).
	if s.filter != nil && filepath.Ext(cleaned) != "" {
		if !s.filter.IsAllowedExtension(cleaned) {
			return "", &PathError{Op: "resolve", Path: relativePath, Err: ErrPathRestricted}
		}
	}

	// Existence check with case-insensitive fallback.
	finalAbs, err := s.existenceCheck(relativePath, absPath)
	if err != nil {
		return "", err
	}

	// Symlink check.
	return s.resolveSymlink("resolve", relativePath, finalAbs)
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
// It uses a single file descriptor for consistent size/modTime vs content.
func (s *Service) ReadNote(ctx context.Context, path string) (*Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	absPath, err := s.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	data, err := io.ReadAll(f)
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

	_, absPath, err := s.sanitizePath("write", path)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(absPath)

	// Symlink escape check on parent directory (if it exists).
	// Uses os.Stat to follow symlinks — a symlinked parent resolves to its target.
	if _, statErr := os.Stat(parentDir); statErr == nil {
		if _, err := s.resolveSymlink("write", path, parentDir); err != nil {
			return err
		}
	}

	// Symlink escape check on the file itself (if it exists as a symlink).
	if info, statErr := os.Lstat(absPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := s.resolveSymlink("write", path, absPath); err != nil {
			return err
		}
	}

	// Lock for all write modes to prevent concurrent write data loss.
	s.mu.Lock()
	defer s.mu.Unlock()

	switch mode {
	case WriteModeOverwrite:
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			return &PathError{Op: "write", Path: path, Err: err}
		}

	case WriteModeAppend, WriteModePrepend:
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
		return &PathError{Op: "write", Path: path, Err: fmt.Errorf("unknown write mode: %q", mode)}
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
		_, resolved, err := s.sanitizePath("list", path)
		if err != nil {
			return nil, err
		}
		absPath = resolved

		// Existence check.
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return nil, &PathError{Op: "list", Path: path, Err: ErrNotFound}
			}
			return nil, &PathError{Op: "list", Path: path, Err: err}
		}

		// Symlink escape check (consistent with ResolvePath step 4).
		symlinkResolved, err := s.resolveSymlink("list", path, absPath)
		if err != nil {
			return nil, err
		}
		absPath = symlinkResolved
	}

	rawEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, &PathError{Op: "list", Path: path, Err: err}
	}

	results := make([]DirEntry, 0, len(rawEntries))
	for _, entry := range rawEntries {
		name := entry.Name()

		// Compute relPath first so filter checks the full relative path.
		var relPath string
		if path == "" {
			relPath = name
		} else {
			relPath = filepath.Join(path, name)
		}

		// Skip ignored entries using the full relative path.
		if s.filter != nil && s.filter.IsIgnored(relPath) {
			continue
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

// NoteInfo holds lightweight metadata for a note (no content returned to caller).
type NoteInfo struct {
	Path      string // vault-relative path, forward slashes
	Size      int64
	ModTime   time.Time
	Title     string // frontmatter "title" key, or filename stem as fallback
	TagCount  int    // number of unique tags from ListTags
	LinkCount int    // number of unique link targets from ExtractLinks
}

// StatNote returns NoteInfo for the given vault-relative path.
// It reads the full note content (needed for tags and links) but does NOT
// return the content itself.
//
// ReadNote is called exactly once; frontmatter and tags are extracted from
// the already-read content so there is no second I/O round-trip.
func (s *Service) StatNote(ctx context.Context, path string) (*NoteInfo, error) {
	note, err := s.ReadNote(ctx, path)
	if err != nil {
		return nil, err
	}

	// Parse frontmatter directly from the already-read content (no second read).
	rawFM, body, hasFM := SplitFrontmatter(note.Content)

	// Parse frontmatter once; extract title and tags from the same map.
	title := ""
	var fmTags []string
	if hasFM {
		if fm, fmErr := ParseFrontmatter(rawFM); fmErr == nil {
			if v, ok := fm["title"]; ok {
				if titleStr, ok := v.(string); ok {
					title = titleStr
				}
			}
			fmTags = ExtractFrontmatterTags(fm)
		}
	}
	// Fall back to filename stem.
	if title == "" {
		base := filepath.Base(path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	inlineTags := ExtractInlineTags(body)

	// Merge: frontmatter tags first, then inline-only tags.
	seen := make(map[string]struct{}, len(fmTags)+len(inlineTags))
	tags := make([]string, 0, len(fmTags)+len(inlineTags))
	for _, t := range fmTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}
	for _, t := range inlineTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}

	// Links: extract from the full note content (wikilinks span the whole file).
	links := ExtractLinks(note.Content)

	return &NoteInfo{
		Path:      path,
		Size:      note.Size,
		ModTime:   note.ModTime,
		Title:     title,
		TagCount:  len(tags),
		LinkCount: len(links),
	}, nil
}
