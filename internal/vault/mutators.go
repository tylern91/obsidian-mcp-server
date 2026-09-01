package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
func (s *Service) PatchNote(ctx context.Context, path string, p PatchOp, opts ...WriteOpt) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "patch", Path: path, Err: err}
	}

	o := applyWriteOpts(opts)

	_, absPath, err := s.sanitizePath("patch", path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("patch", path, absPath); err != nil {
		return err
	}

	data, _, err := readNoteBytes(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: "patch", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "patch", Path: path, Err: err}
	}

	if err := checkIfMatch("patch", path, data, o); err != nil {
		return err
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
// A leading '#' run must be followed by a space to count as a heading (ATX
// heading syntax), matching headingLevel's stricter rule — otherwise a
// malformed line like "#Introduction" would match here but produce an empty
// level from headingLevel, breaking the same-or-higher-level scan in PatchNote.
func isHeadingLine(line, heading string) bool {
	return headingLevel(line) != "" && strings.TrimSpace(strings.TrimLeft(line, "#")) == heading
}

// headingLevel returns the '#' prefix of a heading line, or "" if not a heading.
func headingLevel(line string) string {
	trimmed := strings.TrimLeft(line, "#")
	if len(trimmed) == len(line) || (len(trimmed) > 0 && trimmed[0] != ' ') {
		return ""
	}
	return line[:len(line)-len(trimmed)]
}

// splitLines splits content into lines, removing any trailing empty element
// produced by a trailing newline.
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

// DeleteNote removes a note from the vault. By default it moves the note to
// the trash (.obsidian-mcp/trash/<timestamp>/<path>, preserving vault-relative
// structure) so an accidental delete is recoverable; permanent=true hard-deletes
// instead. confirm must equal path exactly, otherwise ErrConfirmMismatch is
// returned — this guard applies regardless of permanent.
func (s *Service) DeleteNote(ctx context.Context, path, confirm string, permanent bool, opts ...WriteOpt) error {
	if err := ctx.Err(); err != nil {
		return &PathError{Op: "delete", Path: path, Err: err}
	}

	if confirm != path {
		return &PathError{Op: "delete", Path: path, Err: ErrConfirmMismatch}
	}

	o := applyWriteOpts(opts)

	cleaned, absPath, err := s.sanitizePath("delete", path)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("delete", path, absPath); err != nil {
		return err
	}

	if o.ifMatch != "" {
		data, _, readErr := readNoteBytes(absPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return &PathError{Op: "delete", Path: path, Err: ErrNotFound}
			}
			return &PathError{Op: "delete", Path: path, Err: readErr}
		}
		if err := checkIfMatch("delete", path, data, o); err != nil {
			return err
		}
	}

	if permanent {
		if err := os.Remove(absPath); err != nil {
			if os.IsNotExist(err) {
				return &PathError{Op: "delete", Path: path, Err: ErrNotFound}
			}
			return &PathError{Op: "delete", Path: path, Err: err}
		}
		return nil
	}

	trashDst := filepath.Join(s.root, internalStateDir, "trash", trashTimestamp(), cleaned)
	if err := os.MkdirAll(filepath.Dir(trashDst), 0755); err != nil {
		return &PathError{Op: "delete", Path: path, Err: err}
	}
	if err := os.Rename(absPath, trashDst); err != nil {
		if os.IsNotExist(err) {
			return &PathError{Op: "delete", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "delete", Path: path, Err: err}
	}
	return nil
}

// trashTimestamp returns a filesystem-safe, lexically-sortable timestamp for
// a new trash entry directory: UTC, nanosecond precision, no colons (so the
// path is valid on Windows too).
func trashTimestamp() string {
	return time.Now().UTC().Format(trashTimestampLayout)
}

// trashTimestampLayout is shared by trashTimestamp (format) and PruneTrash
// (parse) — a trash entry directory name is exactly this layout.
const trashTimestampLayout = "20060102T150405.000000000"

// PruneTrash removes trash entry directories older than retentionDays,
// relative to now. Entries are the timestamp-named directories DeleteNote
// creates directly under .obsidian-mcp/trash; a directory name that doesn't
// parse as trashTimestampLayout is left alone rather than guessed at, since
// it wasn't created by this code. Returns the number of entries removed.
// Intended to run once at startup (best-effort — see main.go), not on a
// timer, since a long-lived MCP server process is the exception rather than
// the norm for this transport.
func (s *Service) PruneTrash(now time.Time, retentionDays int) (int, error) {
	trashRoot := filepath.Join(s.root, internalStateDir, "trash")
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, &PathError{Op: "prune-trash", Path: trashRoot, Err: err}
	}

	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ts, err := time.Parse(trashTimestampLayout, entry.Name())
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(trashRoot, entry.Name())); err != nil {
				return removed, &PathError{Op: "prune-trash", Path: entry.Name(), Err: err}
			}
			removed++
		}
	}
	return removed, nil
}

// MoveResult is the outcome of MoveNote, including any inbound-link rewrites.
type MoveResult struct {
	Src, Dst string
	Moved    bool          // false only when dryRun
	Links    []LinkRewrite // link rewrites found/applied; nil if updateLinks is false
}

// MoveNote moves a note from src to dst within the vault.
// confirm must equal src exactly, otherwise ErrConfirmMismatch is returned.
// Returns ErrAlreadyExists if dst already exists.
// Note: confirm binds to src only; a typo in dst moves the note to the wrong location.
//
// When updateLinks is true, inbound links to src found elsewhere in the
// vault are rewritten to point at dst (see RewriteLinksOnMove) — ambiguous
// targets are reported, never guessed at. When dryRun is true, nothing on
// disk changes (no move, no link rewrite) — the result previews what would
// happen.
// Note: dryRun does not enforce WriteOpt.ifMatch — the dry-run branch is a
// lock-free os.Stat preview by design (see below), so there is no critical
// section in which to compare content safely. Only the real move enforces it.
func (s *Service) MoveNote(ctx context.Context, src, dst, confirm string, updateLinks, dryRun bool, opts ...WriteOpt) (*MoveResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "move", Path: src, Err: err}
	}

	if confirm != src {
		return nil, &PathError{Op: "move", Path: src, Err: ErrConfirmMismatch}
	}

	_, srcAbs, err := s.sanitizePath("move", src)
	if err != nil {
		return nil, err
	}

	_, dstAbs, err := s.sanitizePath("move", dst)
	if err != nil {
		return nil, err
	}

	if dryRun {
		if _, statErr := os.Stat(dstAbs); statErr == nil {
			return nil, &PathError{Op: "move", Path: dst, Err: ErrAlreadyExists}
		}
		if _, statErr := os.Stat(srcAbs); statErr != nil {
			if os.IsNotExist(statErr) {
				return nil, &PathError{Op: "move", Path: src, Err: ErrNotFound}
			}
			return nil, &PathError{Op: "move", Path: src, Err: statErr}
		}
	} else if err := s.moveFileLocked(src, srcAbs, dst, dstAbs, opts...); err != nil {
		return nil, err
	}

	result := &MoveResult{Src: src, Dst: dst, Moved: !dryRun}
	if updateLinks {
		links, linkErr := s.RewriteLinksOnMove(ctx, src, dst, dryRun)
		if linkErr != nil {
			return result, linkErr
		}
		result.Links = links
	}
	return result, nil
}

// moveFileLocked performs the actual rename under s.mu, preserving the
// existing dst-exists-check-then-rename critical section: two concurrent
// moves to the same dst must not both pass the check and both os.Rename,
// silently clobbering one of the notes.
func (s *Service) moveFileLocked(src, srcAbs, dst, dstAbs string, opts ...WriteOpt) error {
	o := applyWriteOpts(opts)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, statErr := os.Stat(dstAbs); statErr == nil {
		return &PathError{Op: "move", Path: dst, Err: ErrAlreadyExists}
	}

	if err := s.checkSymlinksForWrite("move", src, srcAbs); err != nil {
		return err
	}
	if err := s.checkSymlinksForWrite("move", dst, dstAbs); err != nil {
		return err
	}

	if o.ifMatch != "" {
		data, _, readErr := readNoteBytes(srcAbs)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return &PathError{Op: "move", Path: src, Err: ErrNotFound}
			}
			return &PathError{Op: "move", Path: src, Err: readErr}
		}
		if err := checkIfMatch("move", src, data, o); err != nil {
			return err
		}
	}

	dstParent := filepath.Dir(dstAbs)
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
