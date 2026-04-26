package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PatchOp describes a heading-anchored patch operation.
type PatchOp struct {
	Heading  string // heading text without '#' prefix (e.g. "Introduction")
	Position string // "before" | "after" | "replace_body"
	Content  string // content to insert or use as replacement
}

// PatchNote applies a heading-anchored patch to a note.
// Position "before" inserts content before the heading line.
// Position "after" inserts content after the heading's body (before the next same-level heading).
// Position "replace_body" replaces the body of the heading section.
func (s *Service) PatchNote(ctx context.Context, path string, p PatchOp) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "patch", Path: path, Err: err}
	}

	_, absPath, err := s.sanitizePath("patch", path)
	if err != nil {
		return err
	}

	// Symlink checks before locking.
	parentDir := filepath.Dir(absPath)
	if _, statErr := os.Stat(parentDir); statErr == nil {
		if _, err := s.resolveSymlink("patch", path, parentDir); err != nil {
			return err
		}
	}
	if info, statErr := os.Lstat(absPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := s.resolveSymlink("patch", path, absPath); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: "patch", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "patch", Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")

	// Find the heading line.
	headingIdx := -1
	var headingPrefix string
	for i, line := range lines {
		if isHeadingLine(line, p.Heading) {
			headingIdx = i
			headingPrefix = headingLevel(line)
			break
		}
	}
	if headingIdx == -1 {
		return &PathError{Op: "patch", Path: path, Err: ErrHeadingNotFound}
	}

	// Find the end of the heading's body: next heading of same or higher level, or EOF.
	bodyEnd := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		lvl := headingLevel(lines[i])
		if lvl != "" && len(lvl) <= len(headingPrefix) {
			bodyEnd = i
			break
		}
	}

	var result []string
	switch p.Position {
	case "before":
		result = append(lines[:headingIdx:headingIdx], append(splitLines(p.Content), lines[headingIdx:]...)...)
	case "after":
		result = append(lines[:bodyEnd:bodyEnd], append(splitLines(p.Content), lines[bodyEnd:]...)...)
	case "replace_body":
		// Replace lines from headingIdx+1 to bodyEnd with new content.
		replacement := splitLines(p.Content)
		result = make([]string, 0, headingIdx+1+len(replacement)+(len(lines)-bodyEnd))
		result = append(result, lines[:headingIdx+1]...)
		result = append(result, replacement...)
		result = append(result, lines[bodyEnd:]...)
	default:
		return &PathError{Op: "patch", Path: path, Err: fmt.Errorf("unknown position: %q", p.Position)}
	}

	combined := strings.Join(result, "\n")
	if err := os.WriteFile(absPath, []byte(combined), 0644); err != nil {
		return &PathError{Op: "patch", Path: path, Err: err}
	}
	return nil
}

// isHeadingLine reports whether line is a markdown heading with the given text.
func isHeadingLine(line, heading string) bool {
	trimmed := strings.TrimLeft(line, "#")
	if len(trimmed) == len(line) {
		return false // no leading #
	}
	return strings.TrimSpace(trimmed) == heading
}

// headingLevel returns the '#' prefix of a heading line, or "" if not a heading.
func headingLevel(line string) string {
	trimmed := strings.TrimLeft(line, "#")
	if len(trimmed) == len(line) || (len(trimmed) > 0 && trimmed[0] != ' ') {
		return ""
	}
	return line[:len(line)-len(trimmed)]
}

// splitLines splits content into lines, preserving the trailing-newline convention.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	parts := strings.Split(content, "\n")
	// Remove trailing empty string caused by a trailing newline.
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// DeleteNote deletes a note from the vault.
// confirm must equal path exactly, otherwise ErrConfirmMismatch is returned.
func (s *Service) DeleteNote(ctx context.Context, path, confirm string) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "delete", Path: path, Err: err}
	}

	if confirm != path {
		return &PathError{Op: "delete", Path: path, Err: ErrConfirmMismatch}
	}

	_, absPath, err := s.sanitizePath("delete", path)
	if err != nil {
		return err
	}

	// Symlink escape check on parent dir.
	parentDir := filepath.Dir(absPath)
	if _, statErr := os.Stat(parentDir); statErr == nil {
		if _, err := s.resolveSymlink("delete", path, parentDir); err != nil {
			return err
		}
	}

	// Symlink escape check on the file itself.
	if info, statErr := os.Lstat(absPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := s.resolveSymlink("delete", path, absPath); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(absPath); err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: "delete", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "delete", Path: path, Err: err}
	}
	return nil
}

// MoveNote moves a note from src to dst within the vault.
// confirm must equal src exactly, otherwise ErrConfirmMismatch is returned.
// Returns ErrAlreadyExists if dst already exists.
func (s *Service) MoveNote(ctx context.Context, src, dst, confirm string) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "move", Path: src, Err: err}
	}

	if confirm != src {
		return &PathError{Op: "move", Path: src, Err: ErrConfirmMismatch}
	}

	_, srcAbs, err := s.sanitizePath("move", src)
	if err != nil {
		return err
	}

	_, dstAbs, err := s.sanitizePath("move", dst)
	if err != nil {
		return err
	}

	// Check dst does not already exist.
	if _, statErr := os.Stat(dstAbs); statErr == nil {
		return &PathError{Op: "move", Path: dst, Err: ErrAlreadyExists}
	}

	// Symlink check on src parent.
	srcParent := filepath.Dir(srcAbs)
	if _, statErr := os.Stat(srcParent); statErr == nil {
		if _, err := s.resolveSymlink("move", src, srcParent); err != nil {
			return err
		}
	}

	// Symlink check on src file itself.
	if info, statErr := os.Lstat(srcAbs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if _, err := s.resolveSymlink("move", src, srcAbs); err != nil {
			return err
		}
	}

	// Symlink check on dst parent (if it exists).
	dstParent := filepath.Dir(dstAbs)
	if _, statErr := os.Stat(dstParent); statErr == nil {
		if _, err := s.resolveSymlink("move", dst, dstParent); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(dstParent, 0755); err != nil {
		return &PathError{Op: "move", Path: dst, Err: err}
	}

	if err := os.Rename(srcAbs, dstAbs); err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: "move", Path: src, Err: ErrNotFound}
		}
		return &PathError{Op: "move", Path: src, Err: err}
	}
	return nil
}

// Root returns the symlink-resolved absolute path to the vault root.
func (s *Service) Root() string {
	return s.root
}
