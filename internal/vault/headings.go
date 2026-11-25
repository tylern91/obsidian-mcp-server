package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
)

// headingLoc is one ATX heading found by scanHeadings.
type headingLoc struct {
	idx    int    // 0-based line index
	prefix string // the '#' run
	text   string // heading text, trimmed
}

// fenceMask reports, per line, whether the line is part of a fenced code
// block (opening fence, body, and closing fence included).
func fenceMask(lines []string) []bool {
	mask := make([]bool, len(lines))
	inFence := false
	var fenceChar byte
	var fenceLen int
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if inFence {
			mask[i] = true
			if markdown.IsFenceCloser(trimmed, fenceChar, fenceLen) {
				inFence = false
			}
			continue
		}
		if ch, n, ok := markdown.FenceOpener(trimmed); ok {
			inFence = true
			fenceChar, fenceLen = ch, n
			mask[i] = true
		}
	}
	return mask
}

// scanHeadings returns every ATX heading outside fenced code blocks, in
// document order. It is the one heading parser shared by PatchNote,
// RenameHeading and GetNoteOutline, so they never disagree about what counts
// as a heading.
func scanHeadings(lines []string) []headingLoc {
	mask := fenceMask(lines)
	var out []headingLoc
	for i, line := range lines {
		if mask[i] {
			continue
		}
		prefix := headingLevel(line)
		if prefix == "" {
			continue
		}
		out = append(out, headingLoc{idx: i, prefix: prefix, text: strings.TrimSpace(line[len(prefix):])})
	}
	return out
}

// selectHeading picks the heading whose text equals text. With line > 0 it
// must be the heading on that 1-based line. With line == 0 exactly one
// heading may match; several matches return ErrAmbiguousHeading listing each
// as "line N (level K)". A non-zero line that is not a matching heading, and
// a text with no match, return ErrHeadingNotFound.
func selectHeading(headings []headingLoc, text string, line int) (headingLoc, error) {
	var matches []headingLoc
	for _, h := range headings {
		if h.text == text {
			matches = append(matches, h)
		}
	}
	if line != 0 {
		for _, h := range matches {
			if h.idx+1 == line {
				return h, nil
			}
		}
		return headingLoc{}, ErrHeadingNotFound
	}
	switch len(matches) {
	case 0:
		return headingLoc{}, ErrHeadingNotFound
	case 1:
		return matches[0], nil
	}
	parts := make([]string, len(matches))
	for i, h := range matches {
		parts[i] = fmt.Sprintf("line %d (level %d)", h.idx+1, len(h.prefix))
	}
	return headingLoc{}, fmt.Errorf("%w: %q matches %s; pass line to choose one",
		ErrAmbiguousHeading, text, strings.Join(parts, ", "))
}

// sectionEnd returns the index of the first line after h's section: the next
// heading of the same or higher level outside code fences, or len(lines).
func sectionEnd(lines []string, headings []headingLoc, h headingLoc) int {
	for _, n := range headings {
		if n.idx > h.idx && len(n.prefix) <= len(h.prefix) {
			return n.idx
		}
	}
	return len(lines)
}

// RenameHeadingResult is the outcome of RenameHeading.
type RenameHeadingResult struct {
	Path       string
	OldHeading string
	NewHeading string
	Level      int
	Line       int           // 1-based line of the renamed heading
	Links      []LinkRewrite // anchor rewrites found/applied; nil when updateLinks is false or skipped
	// LinksSkipped is non-empty when updateLinks was requested but links were
	// left alone, with the reason.
	LinksSkipped string
}

// selfAnchorRegex matches same-note wikilinks "[[#Heading]]" / "[[#A#Heading|alias]]".
var selfAnchorRegex = regexp.MustCompile(`(!?)\[\[#([^\]|]*)(\|[^\]]*)?\]\]`)

// RenameHeading rewrites one heading's text in a note, keeping its level.
// heading/line select the heading exactly as PatchOp does (ambiguity is an
// error unless line is given). With updateLinks, anchors that point at the
// old heading ([[Note#Old]], [[Note#Old|alias]], [text](Note.md#old), and
// same-note [[#Old]]) are rewritten vault-wide through the move_note link
// machinery; ambiguous link targets are reported and left unchanged. When
// the note has an earlier heading with the same old text, links keep
// resolving to that one, so they are left alone and LinksSkipped says why.
func (s *Service) RenameHeading(ctx context.Context, path, heading, newHeading string, line int, updateLinks bool, opts ...WriteOpt) (*RenameHeadingResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "rename-heading", Path: path, Err: err}
	}
	newHeading = strings.TrimSpace(newHeading)
	if newHeading == "" || strings.ContainsAny(newHeading, "\r\n") {
		return nil, &PathError{Op: "rename-heading", Path: path, Err: errors.New("newHeading must be non-empty single-line text")}
	}

	o := applyWriteOpts(opts)
	cleaned, absPath, err := s.sanitizePath("rename-heading", path)
	if err != nil {
		return nil, err
	}

	res, err := s.renameHeadingLocked(path, absPath, heading, newHeading, line, updateLinks, o)
	if err != nil {
		return nil, err
	}
	if !updateLinks {
		return res, nil
	}
	if res.LinksSkipped != "" {
		return res, nil
	}

	mv := &moveCtx{
		targets: buildMatchTargets(cleaned),
		oldExt:  filepath.Ext(filepath.ToSlash(cleaned)),
		anchor:  &anchorChange{oldText: res.OldHeading, newText: res.NewHeading},
	}
	index, err := s.basenameIndex(ctx)
	if err != nil {
		return res, err
	}
	mv.index = index
	links, linkErr := s.rewriteLinksVaultWide(ctx, mv, "", false)
	res.Links = links
	return res, linkErr
}

// renameHeadingLocked performs the heading edit (and same-note anchor fix)
// under s.mu in the standard sanitize -> lock -> symlink check -> if_match ->
// atomic write order.
func (s *Service) renameHeadingLocked(path, absPath, heading, newHeading string, line int, updateLinks bool, o writeOpts) (*RenameHeadingResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("rename-heading", path, absPath); err != nil {
		return nil, err
	}
	data, _, err := readNoteBytes(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &PathError{Op: "rename-heading", Path: path, Err: ErrNotFound}
		}
		return nil, &PathError{Op: "rename-heading", Path: path, Err: err}
	}
	if err := checkIfMatch("rename-heading", path, data, o); err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	headings := scanHeadings(lines)
	target, err := selectHeading(headings, heading, line)
	if err != nil {
		return nil, &PathError{Op: "rename-heading", Path: path, Err: err}
	}

	res := &RenameHeadingResult{
		Path: path, OldHeading: heading, NewHeading: newHeading,
		Level: len(target.prefix), Line: target.idx + 1,
	}
	if first, _ := selectHeadingFirst(headings, heading); first.idx != target.idx {
		res.LinksSkipped = "an earlier heading with the same text remains the link target"
	}

	eol := ""
	if strings.HasSuffix(lines[target.idx], "\r") {
		eol = "\r"
	}
	lines[target.idx] = target.prefix + " " + newHeading + eol

	// Same-note anchors [[#Old]]: not reachable through the vault-wide walk
	// (empty link target), so fix them in this same write, only when the
	// caller asked for link updates.
	if updateLinks && res.LinksSkipped == "" {
		mask := fenceMask(lines)
		for i, l := range lines {
			if mask[i] || !strings.Contains(l, "[[#") {
				continue
			}
			lines[i] = selfAnchorRegex.ReplaceAllStringFunc(l, func(m string) string {
				sm := selfAnchorRegex.FindStringSubmatch(m)
				parts := strings.Split(sm[2], "#")
				last := parts[len(parts)-1]
				if strings.HasPrefix(last, "^") || !strings.EqualFold(strings.TrimSpace(last), heading) {
					return m
				}
				parts[len(parts)-1] = newHeading
				return sm[1] + "[[#" + strings.Join(parts, "#") + sm[3] + "]]"
			})
		}
	}

	combined := strings.Join(lines, "\n")
	if int64(len(combined)) > maxFileSizeBytes {
		return nil, &PathError{Op: "rename-heading", Path: path, Err: ErrFileTooLarge}
	}
	if err := s.writeFileAtomic(absPath, []byte(combined)); err != nil {
		return nil, &PathError{Op: "rename-heading", Path: path, Err: err}
	}
	return res, nil
}

// selectHeadingFirst returns the first heading with the given text.
func selectHeadingFirst(headings []headingLoc, text string) (headingLoc, bool) {
	for _, h := range headings {
		if h.text == text {
			return h, true
		}
	}
	return headingLoc{}, false
}
