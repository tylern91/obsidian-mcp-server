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

// maxFileSizeBytes caps the size of a note that ReadNote will load into memory.
const maxFileSizeBytes int64 = 16 * 1024 * 1024 // 16 MB

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

	if err := checkWindowsSafe(cleaned); err != nil {
		return "", "", &PathError{Op: op, Path: relativePath, Err: err}
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

// windowsReservedNames are device names that Windows treats specially regardless
// of extension (e.g. "NUL.md" still refers to the NUL device).
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// checkWindowsSafe rejects path forms that are dangerous or ambiguous on Windows
// (drive-relative paths, NTFS alternate data streams, reserved device names, and
// trailing dots/spaces that Windows silently strips), regardless of the platform
// this binary is built for. filepath.IsAbs and filepath.Clean alone don't catch
// these: e.g. "C:foo" is relative per filepath.IsAbs on Windows, and a build for
// one GOOS must still refuse path forms unsafe on a Windows deployment.
func checkWindowsSafe(cleaned string) error {
	segments := strings.Split(cleaned, string(filepath.Separator))
	for _, seg := range segments {
		if seg == "" || seg == "." {
			continue
		}

		if len(seg) >= 2 && seg[1] == ':' && isASCIILetter(seg[0]) {
			return ErrPathTraversal // drive-relative path, e.g. "C:foo"
		}

		if strings.Contains(seg, ":") {
			return ErrPathTraversal // NTFS alternate data stream, e.g. "note.md:hidden"
		}

		name := seg
		if i := strings.IndexByte(name, '.'); i >= 0 {
			name = name[:i]
		}
		if windowsReservedNames[strings.ToUpper(name)] {
			return ErrPathTraversal
		}

		if seg != strings.TrimRight(seg, ". ") {
			return ErrPathTraversal // trailing dot/space, silently stripped by Windows
		}
	}
	return nil
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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

// checkSymlinksForWrite verifies that neither the parent directory nor the file
// itself (when it exists as a symlink) escape the vault boundary.
// Must be called while holding s.mu.
//
// s.mu only serializes this process: an external actor with local filesystem
// write access to the vault could still swap a path component between this
// check and the write syscall that follows it (TOCTOU). Closing that fully
// would require opening with O_NOFOLLOW instead of stat-then-write. This is
// accepted as low-severity — the attacker already needs vault write access —
// and is documented as a known boundary in SECURITY.md.
func (s *Service) checkSymlinksForWrite(op, path, absPath string) error {
	parentDir := filepath.Dir(absPath)
	if _, statErr := os.Stat(parentDir); statErr == nil {
		if _, err := s.resolveSymlink(op, path, parentDir); err != nil {
			return err
		}
	}
	if info, statErr := os.Lstat(absPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := s.resolveSymlink(op, path, absPath); err != nil {
			return err
		}
	}
	return nil
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
// Read paths only (ResolvePath) — write paths must use existsStrict instead,
// since the case-insensitive fallback would make a write target ambiguous.
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

// existsStrict verifies absPath exists via a single os.Stat, with no
// case-insensitive fallback. Write-path prologues must call this — not
// existenceCheck — so a write target is never resolved ambiguously.
// Must be called while holding s.mu, so the check and the write it guards
// don't race a concurrent mutation.
func (s *Service) existsStrict(op, relativePath, absPath string) error {
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: op, Path: relativePath, Err: ErrNotFound}
		}
		return &PathError{Op: op, Path: relativePath, Err: err}
	}
	return nil
}

// readNoteBytes reads absPath into memory via a single file descriptor,
// enforcing maxFileSizeBytes. Returns ErrFileTooLarge if the file exceeds the
// cap. The returned os.FileInfo comes from the same fd as the content, so
// size/modTime stay consistent with what was actually read.
func readNoteBytes(absPath string) ([]byte, os.FileInfo, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}

	if info.Size() > maxFileSizeBytes {
		return nil, nil, ErrFileTooLarge
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	return data, info, nil
}

// ReadNote reads a note at the given relative path and returns its content and metadata.
// Returns ErrFileTooLarge if the file exceeds maxFileSizeBytes.
func (s *Service) ReadNote(ctx context.Context, path string) (*Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "read", Path: path, Err: err}
	}

	absPath, err := s.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, info, err := readNoteBytes(absPath)
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
// Returns ErrFileTooLarge if the content exceeds maxFileSizeBytes.
func (s *Service) WriteNote(ctx context.Context, path, content string, mode WriteMode) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "write", Path: path, Err: err}
	}

	// Reject oversized content before acquiring the lock.
	if int64(len(content)) > maxFileSizeBytes {
		return &PathError{Op: "write", Path: path, Err: ErrFileTooLarge}
	}

	_, absPath, err := s.sanitizePath("write", path)
	if err != nil {
		return err
	}

	// Lock for all write modes to prevent concurrent write data loss.
	// Symlink checks are performed inside the lock to close the TOCTOU window.
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("write", path, absPath); err != nil {
		return err
	}

	parentDir := filepath.Dir(absPath)

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

		if int64(len(combined)) > maxFileSizeBytes {
			return &PathError{Op: "write", Path: path, Err: ErrFileTooLarge}
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
		// Respect context cancellation; stop appending on a cancelled call.
		if err := ctx.Err(); err != nil {
			return nil, &PathError{Op: "list", Path: path, Err: err}
		}

		name := entry.Name()

		var relPath string
		if path == "" {
			relPath = name
		} else {
			relPath = filepath.Join(path, name)
		}

		if s.filter != nil && s.filter.IsIgnored(relPath) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			// Skip entries whose metadata is unreadable (e.g. broken symlinks,
			// race-removed files). The caller sees a partial listing but no
			// hard error — consistent with best-effort directory enumeration.
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
	rawFM, _, hasFM := SplitFrontmatter(note.Content)

	// Extract title from frontmatter; fall back to filename stem.
	title := ""
	if hasFM {
		if fm, fmErr := ParseFrontmatter(rawFM); fmErr == nil {
			if v, ok := fm["title"]; ok {
				if titleStr, ok := v.(string); ok {
					title = titleStr
				}
			}
		}
	}
	if title == "" {
		title = Stem(path)
	}

	tags := MergeNoteTags([]byte(note.Content))

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
