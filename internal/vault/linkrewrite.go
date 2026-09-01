package vault

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// wikilinkComponentRegex splits an Obsidian wikilink/embed into its parts so
// RewriteLinksOnMove can rewrite the target while preserving everything
// else verbatim. Groups: 1=embed bang ("!" or ""), 2=target, 3=anchor
// (leading "#", covers both heading anchors like "#Intro" and block
// references like "#^abc123" — both are opaque suffixes to this code, never
// interpreted, only preserved), 4=alias (leading "|").
var wikilinkComponentRegex = regexp.MustCompile(`(!?)\[\[([^\]|#]+?)((?:#[^\]|]*)?)(\|[^\]]*)?\]\]`)

// markdownLinkComponentRegex splits a Markdown link to a .md file into its
// parts. Groups: 1="[text](" prefix, 2=path (must end in .md; may be
// URL-encoded), 3=fragment (leading "#", optional), 4=closing ")".
var markdownLinkComponentRegex = regexp.MustCompile(`(\[[^\]]*\]\()([^)#]+\.md)((?:#[^)]*)?)(\))`)

// LinkRewrite describes one link RewriteLinksOnMove found pointing at the
// moved note. NewText is empty when Ambiguous is true — nothing was written
// for that occurrence.
type LinkRewrite struct {
	Path      string // vault-relative path of the note containing the link
	Line      int    // 1-indexed line number
	OldText   string // the original link as it appeared
	NewText   string // the rewritten link (empty if Ambiguous)
	Ambiguous bool   // true if this target string also matches another note
}

// buildMatchTargets returns the set of link-target strings that refer to the
// note at relPath: its basename with and without extension, and its
// vault-relative path (forward-slash) with and without extension.
func buildMatchTargets(relPath string) map[string]bool {
	relSlash := filepath.ToSlash(relPath)
	base := filepath.Base(relSlash)
	baseNoExt := Stem(relSlash)
	relNoExt := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
	return map[string]bool{baseNoExt: true, base: true, relNoExt: true, relSlash: true}
}

// rewriteTarget computes the new link target string for oldTarget (as
// captured from the source markup) once the note it points to has moved to
// newPath, preserving oldTarget's style: a bare-basename target rewrites to
// the new basename; a path-style target (contains "/") rewrites to the new
// vault-relative path. Extension presence in the original is preserved.
func rewriteTarget(oldTarget, newPath string) string {
	newSlash := filepath.ToSlash(newPath)
	hadExt := strings.HasSuffix(oldTarget, filepath.Ext(oldTarget)) && filepath.Ext(oldTarget) != ""
	pathStyle := strings.Contains(oldTarget, "/")

	switch {
	case pathStyle && hadExt:
		return newSlash
	case pathStyle:
		return strings.TrimSuffix(newSlash, filepath.Ext(newSlash))
	case hadExt:
		return filepath.Base(newSlash)
	default:
		return Stem(newSlash)
	}
}

// decodeMarkdownPath best-effort URL-decodes a Markdown link path for
// matching purposes. An undecodable path is returned unchanged rather than
// erroring — malformed encoding just means it won't match, which is safe.
func decodeMarkdownPath(path string) string {
	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}
	return path
}

// encodeMarkdownPath re-encodes a rewritten Markdown link path to match the
// original's encoding style: only if the original contained a "%" escape do
// we re-escape (space -> %20 etc.); otherwise the new path is left as literal
// text, matching how most Obsidian vaults write Markdown links.
func encodeMarkdownPath(newPath, originalRaw string) string {
	if !strings.Contains(originalRaw, "%") {
		return newPath
	}
	segments := strings.Split(newPath, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// basenameIndex maps each vault note's extension-stripped basename to every
// vault-relative path sharing that basename, used to detect ambiguous
// bare-basename link targets.
func (s *Service) basenameIndex(ctx context.Context) (map[string][]string, error) {
	index := make(map[string][]string)
	err := s.WalkNotes(ctx, func(rel, abs string) error {
		index[Stem(rel)] = append(index[Stem(rel)], rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

// RewriteLinksOnMove finds every note in the vault linking to oldPath and
// rewrites those links to point at newPath instead. A link whose bare
// target string also matches some other note in the vault is ambiguous —
// it is reported but left untouched, never guessed at. Heading anchors
// (#Intro), block references (#^abc123), and aliases (|alias text) are
// preserved verbatim; only the target/path portion changes. When dryRun is
// true, no files are written — the same list previews what would change.
//
// Read-only when dryRun; otherwise each modified file is written through
// the standard sanitizePath -> lock -> checkSymlinksForWrite ordering, one
// file at a time (not one lock for the whole vault-wide rewrite), matching
// every other write path in this package.
func (s *Service) RewriteLinksOnMove(ctx context.Context, oldPath, newPath string, dryRun bool) ([]LinkRewrite, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "rewrite-links", Path: oldPath, Err: err}
	}

	targets := buildMatchTargets(oldPath)
	index, err := s.basenameIndex(ctx)
	if err != nil {
		return nil, err
	}

	var rewrites []LinkRewrite

	walkErr := s.WalkNotes(ctx, func(rel, abs string) error {
		if rel == filepath.ToSlash(newPath) {
			return nil // the moved note itself was already renamed on disk
		}

		data, err := os.ReadFile(abs)
		if err != nil {
			return nil // unreadable file: skip, don't fail the whole rewrite
		}

		lines := strings.Split(string(data), "\n")
		changed := false

		for i, line := range lines {
			newLine, lineRewrites := rewriteLine(line, targets, index, newPath)
			if len(lineRewrites) == 0 {
				continue
			}
			for _, r := range lineRewrites {
				r.Path = rel
				r.Line = i + 1
				rewrites = append(rewrites, r)
			}
			if newLine != line {
				lines[i] = newLine
				changed = true
			}
		}

		if changed && !dryRun {
			if writeErr := s.writeRewrittenNote(rel, abs, strings.Join(lines, "\n")); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return rewrites, nil
}

// writeRewrittenNote writes newContent to a single note already known (from
// the WalkNotes callback) to exist at abs, following this package's standard
// write ordering: checkSymlinksForWrite and the write syscall both inside
// s.mu.Lock().
func (s *Service) writeRewrittenNote(rel, abs, newContent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("rewrite-links", rel, abs); err != nil {
		return err
	}
	if int64(len(newContent)) > maxFileSizeBytes {
		return &PathError{Op: "rewrite-links", Path: rel, Err: ErrFileTooLarge}
	}
	if err := os.WriteFile(abs, []byte(newContent), 0644); err != nil {
		return &PathError{Op: "rewrite-links", Path: rel, Err: err}
	}
	return nil
}

// rewriteLine scans one line for wikilinks and Markdown links pointing at
// oldPath (per targets) and returns the rewritten line plus one LinkRewrite
// per occurrence found (Path/Line left zero — the caller fills them in).
// Matches are processed right-to-left by byte offset so earlier replacements
// don't invalidate later ones' indices.
func rewriteLine(line string, targets map[string]bool, index map[string][]string, newPath string) (string, []LinkRewrite) {
	type match struct {
		start, end int
		rewrite    LinkRewrite
	}
	var matches []match

	for _, m := range wikilinkComponentRegex.FindAllStringSubmatchIndex(line, -1) {
		target := line[m[4]:m[5]]
		targetSlash := filepath.ToSlash(target)
		targetNoExt := strings.TrimSuffix(targetSlash, filepath.Ext(targetSlash))
		if !targets[targetSlash] && !targets[targetNoExt] {
			continue
		}

		oldText := line[m[0]:m[1]]
		bang, anchor, alias := line[m[2]:m[3]], "", ""
		if m[6] != -1 {
			anchor = line[m[6]:m[7]]
		}
		if m[8] != -1 {
			alias = line[m[8]:m[9]]
		}

		if !strings.Contains(target, "/") && len(index[targetNoExt]) > 1 {
			matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldText, Ambiguous: true}})
			continue
		}

		newTarget := rewriteTarget(target, newPath)
		newText := bang + "[[" + newTarget + anchor + alias + "]]"
		matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldText, NewText: newText}})
	}

	for _, m := range markdownLinkComponentRegex.FindAllStringSubmatchIndex(line, -1) {
		rawPath := line[m[4]:m[5]]
		decoded := decodeMarkdownPath(rawPath)
		decodedNoExt := strings.TrimSuffix(decoded, filepath.Ext(decoded))
		if !targets[decoded] && !targets[decodedNoExt] {
			continue
		}

		oldText := line[m[0]:m[1]]
		prefix := line[m[2]:m[3]]
		fragment := ""
		if m[6] != -1 {
			fragment = line[m[6]:m[7]]
		}
		closer := line[m[8]:m[9]]

		if !strings.Contains(decoded, "/") && len(index[decodedNoExt]) > 1 {
			matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldText, Ambiguous: true}})
			continue
		}

		newTarget := encodeMarkdownPath(rewriteTarget(decoded, newPath), rawPath)
		newText := prefix + newTarget + fragment + closer
		matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldText, NewText: newText}})
	}

	if len(matches) == 0 {
		return line, nil
	}

	rewrites := make([]LinkRewrite, len(matches))
	result := line
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		rewrites[i] = m.rewrite
		if !m.rewrite.Ambiguous {
			result = result[:m.start] + m.rewrite.NewText + result[m.end:]
		}
	}
	return result, rewrites
}
