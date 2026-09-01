package vault

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
)

// TagRename describes one note RenameTag modified.
type TagRename struct {
	Path           string
	FrontmatterHit bool // the frontmatter "tags" sequence contained oldTag
	InlineCount    int  // number of inline #oldTag occurrences replaced
}

// RenameTag renames oldTag to newTag across every note in the vault, in both
// the frontmatter "tags" sequence and inline #tag occurrences. Frontmatter
// edits rename the scalar node in place via the yaml.v3 Node API, so key
// order and every other frontmatter value are preserved — never a naive
// marshal round-trip. Inline occurrences inside fenced code blocks are left
// alone, matching AddTag/RemoveTag's existing convention that a tag inside a
// code block was never a real tag.
//
// A note with neither occurrence is left untouched — not rewritten, not
// counted. Returns one TagRename per note actually modified.
func (s *Service) RenameTag(ctx context.Context, oldTag, newTag string) ([]TagRename, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "rename_tag", Path: oldTag, Err: err}
	}
	if oldTag == "" || newTag == "" {
		return nil, &PathError{Op: "rename_tag", Path: oldTag, Err: fmt.Errorf("oldTag and newTag must both be non-empty")}
	}

	var renames []TagRename

	walkErr := s.WalkNotes(ctx, func(rel, abs string) error {
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil // unreadable: skip, don't fail the whole rename
		}

		newContent, fmHit, inlineCount, err := renameTagInContent(string(data), oldTag, newTag)
		if err != nil {
			return &PathError{Op: "rename_tag", Path: rel, Err: err}
		}
		if !fmHit && inlineCount == 0 {
			return nil
		}

		if err := s.writeRenamedTagNote(rel, abs, newContent); err != nil {
			return err
		}
		renames = append(renames, TagRename{Path: rel, FrontmatterHit: fmHit, InlineCount: inlineCount})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return renames, nil
}

// writeRenamedTagNote writes newContent to a single note already known
// (from the WalkNotes callback) to exist at abs.
func (s *Service) writeRenamedTagNote(rel, abs, newContent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("rename_tag", rel, abs); err != nil {
		return err
	}
	if int64(len(newContent)) > maxFileSizeBytes {
		return &PathError{Op: "rename_tag", Path: rel, Err: ErrFileTooLarge}
	}
	if err := os.WriteFile(abs, []byte(newContent), 0644); err != nil {
		return &PathError{Op: "rename_tag", Path: rel, Err: err}
	}
	return nil
}

// renameTagInContent renames oldTag to newTag in content's frontmatter
// "tags" sequence and inline #tag occurrences (outside fenced code blocks).
// Returns the new content, whether the frontmatter sequence was touched, and
// how many inline occurrences were replaced. Returns the original content
// unchanged (with both flags zero) if oldTag was not found anywhere.
func renameTagInContent(content, oldTag, newTag string) (string, bool, int, error) {
	rawFM, body, hasFM := SplitFrontmatter(content)

	fmHit := false
	newRawFM := rawFM
	if hasFM {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(rawFM), &doc); err != nil {
			return "", false, 0, fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			mapping := doc.Content[0]
			for i := 0; i+1 < len(mapping.Content); i += 2 {
				if mapping.Content[i].Value != "tags" {
					continue
				}
				seqNode := mapping.Content[i+1]
				if seqNode.Kind == yaml.SequenceNode {
					for _, child := range seqNode.Content {
						if child.Value == oldTag {
							child.Value = newTag
							fmHit = true
						}
					}
				} else if seqNode.Value == oldTag {
					seqNode.Value = newTag
					fmHit = true
				}
				break
			}
			if fmHit {
				out, err := yaml.Marshal(mapping)
				if err != nil {
					return "", false, 0, fmt.Errorf("marshal: %w", err)
				}
				newRawFM = string(out)
			}
		}
	}

	newBody, inlineCount := renameInlineTag(body, oldTag, newTag)

	if !fmHit && inlineCount == 0 {
		return content, false, 0, nil
	}

	if hasFM {
		return "---\n" + newRawFM + "---\n" + newBody, fmHit, inlineCount, nil
	}
	return newBody, fmHit, inlineCount, nil
}

// renameInlineTag replaces inline #oldTag occurrences with #newTag, skipping
// fenced code blocks via the same fence-state-machine style as RemoveTag's
// inline removal — kept as a separate scanner rather than shared, matching
// this file's existing pattern of near-duplicate fence-aware scanners
// (AddTag/RemoveTag), not introduced by this change.
func renameInlineTag(body, oldTag, newTag string) (string, int) {
	pattern := regexp.MustCompile(`(?m)(^|[^\p{L}\p{N}_/])#` + regexp.QuoteMeta(oldTag) + `([^\p{L}\p{N}_/\-]|$)`)

	lines := strings.Split(body, "\n")
	inFence := false
	var fenceChar byte
	var fenceLen int
	out := make([]string, 0, len(lines))
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		if !inFence {
			if ch, n, ok := markdown.FenceOpener(trimmed); ok {
				inFence = true
				fenceChar = ch
				fenceLen = n
				out = append(out, line)
				continue
			}
			replaced := pattern.ReplaceAllStringFunc(line, func(match string) string {
				sub := pattern.FindStringSubmatch(match)
				count++
				return sub[1] + "#" + newTag + sub[2]
			})
			out = append(out, replaced)
		} else {
			if markdown.IsFenceCloser(trimmed, fenceChar, fenceLen) {
				inFence = false
			}
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n"), count
}
