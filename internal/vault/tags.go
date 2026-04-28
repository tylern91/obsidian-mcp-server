package vault

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
)

// tagRegex matches Obsidian-style inline tags: #tag preceded by start-of-line or
// a non-word character.  The tag body must be Unicode letters, digits,
// underscores, slashes, or hyphens.
var tagRegex = regexp.MustCompile(`(?m)(?:^|[^\p{L}\p{N}_/])#([\p{L}\p{N}_/\-]+)`)

// ExtractInlineTags returns all unique inline #tags found in body text.
// Tags are returned in the order first encountered, case-sensitively deduped
// (Obsidian treats #TODO and #todo as distinct tags).
// Trailing '-' and '/' are trimmed to match Obsidian's own trimming behavior.
//
// Code-fenced regions (``` ... ``` and ~~~ ... ~~~) and inline backtick spans
// are excluded from matching so that tags inside code blocks are not counted.
func ExtractInlineTags(body string) []string {
	stripped := markdown.StripCodeFences(body)
	matches := tagRegex.FindAllStringSubmatch(stripped, -1)

	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))

	for _, m := range matches {
		tag := strings.TrimRight(m[1], "-/")
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; !dup {
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}

	return out
}

// ExtractFrontmatterTags extracts the value of the "tags" key from a parsed
// frontmatter map.  It handles the three common Obsidian formats:
//   - Sequence:  tags: [a, b]    → []any{"a", "b"}
//   - CSV string: tags: "a, b"   → split on comma
//   - Scalar:    tags: a         → single item
func ExtractFrontmatterTags(fm map[string]any) []string {
	raw, ok := fm["tags"]
	if !ok || raw == nil {
		return nil
	}

	var out []string

	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, part := range strings.Split(v, ",") {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
	}

	return out
}

// MergeNoteTags returns the deduplicated union of frontmatter and inline tags in
// note content. Frontmatter tags appear first (in declared order), inline tags
// follow (in document order). Deduplication is exact-string, case-sensitive,
// matching Obsidian's treatment of #TODO and #todo as distinct tags.
//
// Frontmatter parse errors are silently ignored and fall back to inline tags only.
// Callers that need parse-error visibility should call SplitFrontmatter +
// ParseFrontmatter directly.
func MergeNoteTags(content []byte) []string {
	rawFM, body, hasFM := SplitFrontmatter(string(content))
	var fmTags []string
	if hasFM {
		if fm, err := ParseFrontmatter(rawFM); err == nil {
			fmTags = ExtractFrontmatterTags(fm)
		}
	}
	inlineTags := ExtractInlineTags(body)

	seen := make(map[string]struct{}, len(fmTags)+len(inlineTags))
	out := make([]string, 0, len(fmTags)+len(inlineTags))
	for _, t := range fmTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	for _, t := range inlineTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// ListTags returns all tags for the note at path, merging frontmatter tags and
// inline tags.  The result is ordered: frontmatter tags first, then inline tags
// not already present in the frontmatter list.
func (s *Service) ListTags(ctx context.Context, path string) ([]string, error) {
	note, err := s.ReadNote(ctx, path)
	if err != nil {
		return nil, err
	}

	raw, body, hasFM := SplitFrontmatter(note.Content)

	var fmTags []string
	if hasFM {
		fm, parseErr := ParseFrontmatter(raw)
		if parseErr != nil {
			return nil, parseErr
		}
		fmTags = ExtractFrontmatterTags(fm)
	}

	inlineTags := ExtractInlineTags(body)

	// Merge: frontmatter tags first, then inline-only tags.
	seen := make(map[string]struct{}, len(fmTags)+len(inlineTags))
	out := make([]string, 0, len(fmTags)+len(inlineTags))

	for _, t := range fmTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	for _, t := range inlineTags {
		if _, dup := seen[t]; !dup {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}

	return out, nil
}

// AddTag adds tag to the note at path.
//
// location must be "frontmatter" or "inline" (default: "frontmatter").
//
//   - "frontmatter": appends the tag to the YAML "tags" sequence (no-op if
//     already present).
//   - "inline": appends "\n#tag" to the end of the note body.
//
// The method is atomic: it locks the service mutex for the entire
// read-modify-write cycle.
func (s *Service) AddTag(ctx context.Context, path, tag, location string) error {
	if location == "" {
		location = "frontmatter"
	}
	if location != "frontmatter" && location != "inline" {
		return fmt.Errorf("add_tag: invalid location %q: must be frontmatter or inline", location)
	}

	_, absPath, err := s.sanitizePath("add_tag", path)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(absPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return &PathError{Op: "add_tag", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "add_tag", Path: path, Err: statErr}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("add_tag", path, absPath); err != nil {
		return err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &PathError{Op: "add_tag", Path: path, Err: err}
	}

	content := string(data)
	rawFM, body, hasFM := SplitFrontmatter(content)

	var mapping *yaml.Node
	if hasFM {
		var doc yaml.Node
		if unmarshalErr := yaml.Unmarshal([]byte(rawFM), &doc); unmarshalErr != nil {
			return fmt.Errorf("%w: %s", ErrInvalidFrontmatter, unmarshalErr.Error())
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			mapping = doc.Content[0]
		}
	}
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}

	if location == "frontmatter" {
		content, err = addTagToFrontmatter(mapping, tag, body)
		if err != nil {
			return fmt.Errorf("add_tag: %w", err)
		}
	} else {
		// Inline: append \n#tag to the body, then reassemble.
		newBody := body
		if !strings.HasSuffix(newBody, "\n") {
			newBody += "\n"
		}
		newBody += "#" + tag + "\n"

		if hasFM {
			out, marshalErr := yaml.Marshal(mapping)
			if marshalErr != nil {
				return fmt.Errorf("add_tag: marshal: %w", marshalErr)
			}
			content = "---\n" + string(out) + "---\n" + newBody
		} else {
			content = newBody
		}
	}

	if writeErr := os.WriteFile(absPath, []byte(content), 0644); writeErr != nil {
		return &PathError{Op: "add_tag", Path: path, Err: writeErr}
	}

	return nil
}

// addTagToFrontmatter appends tag to the "tags" sequence in mapping (or creates
// the key if absent).  Returns the fully assembled file content on success.
// No-op if the tag is already present.
func addTagToFrontmatter(mapping *yaml.Node, tag, body string) (string, error) {
	// Find the "tags" key in the mapping.
	tagsIdx := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "tags" {
			tagsIdx = i + 1
			break
		}
	}

	tagScalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tag}

	if tagsIdx < 0 {
		// No "tags" key — create a new sequence with just this tag.
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tags"}
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{tagScalar}}
		mapping.Content = append(mapping.Content, keyNode, seqNode)
	} else {
		seqNode := mapping.Content[tagsIdx]

		// Ensure it's a sequence; if it's a scalar, convert it.
		if seqNode.Kind != yaml.SequenceNode {
			existing := seqNode.Value
			seqNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			if existing != "" {
				seqNode.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: existing}}
			}
			mapping.Content[tagsIdx] = seqNode
		}

		// Check for duplicate.
		for _, child := range seqNode.Content {
			if child.Value == tag {
				// Already present — reassemble unchanged.
				out, err := yaml.Marshal(mapping)
				if err != nil {
					return "", fmt.Errorf("marshal: %w", err)
				}
				return "---\n" + string(out) + "---\n" + body, nil
			}
		}

		seqNode.Content = append(seqNode.Content, tagScalar)
	}

	out, err := yaml.Marshal(mapping)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	return "---\n" + string(out) + "---\n" + body, nil
}

// RemoveTag removes all occurrences of tag from both the frontmatter "tags"
// sequence and inline #tag patterns in the note body.
//
// The method is a no-op (returns nil) when the tag is not present in either
// location.  It is atomic: holds the service mutex for the full
// read-modify-write cycle.
//
// Code-fenced regions are skipped during the inline removal: a tag that appears
// only inside a code block is NOT removed from the prose (because it was never
// counted as a prose tag). The body is processed line-by-line using a
// fence-state-machine so that lines inside fences are written back unchanged.
func (s *Service) RemoveTag(ctx context.Context, path, tag string) error {
	_, absPath, err := s.sanitizePath("remove_tag", path)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(absPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return &PathError{Op: "remove_tag", Path: path, Err: ErrNotFound}
		}
		return &PathError{Op: "remove_tag", Path: path, Err: statErr}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkSymlinksForWrite("remove_tag", path, absPath); err != nil {
		return err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return &PathError{Op: "remove_tag", Path: path, Err: err}
	}

	content := string(data)
	rawFM, body, hasFM := SplitFrontmatter(content)

	var mapping *yaml.Node
	if hasFM {
		var doc yaml.Node
		if unmarshalErr := yaml.Unmarshal([]byte(rawFM), &doc); unmarshalErr != nil {
			return fmt.Errorf("%w: %s", ErrInvalidFrontmatter, unmarshalErr.Error())
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			mapping = doc.Content[0]
		}
	}

	// Remove from frontmatter tags sequence.
	if mapping != nil {
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == "tags" {
				seqNode := mapping.Content[i+1]
				if seqNode.Kind == yaml.SequenceNode {
					kept := seqNode.Content[:0]
					for _, child := range seqNode.Content {
						if child.Value != tag {
							kept = append(kept, child)
						}
					}
					seqNode.Content = kept
				}
				break
			}
		}
	}

	// Remove inline #tag occurrences from body, skipping fenced code blocks.
	//
	// We process the body line-by-line with a fence-state-machine identical to
	// markdown.StripCodeFences, but instead of replacing fence content with
	// spaces we preserve it verbatim.  Only lines that are outside a fence have
	// the tag-removal regex applied.
	removeInline := regexp.MustCompile(`(?m)(?:^|([^\p{L}\p{N}_/]))#` + regexp.QuoteMeta(tag) + `(?:[^\p{L}\p{N}_/\-]|$)`)

	lines := strings.Split(body, "\n")
	inFence := false
	var fenceChar byte
	var fenceLen int
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		if !inFence {
			// Check whether this line opens a fence.
			if ch, n := removeTagFenceOpener(trimmed); n > 0 {
				inFence = true
				fenceChar = ch
				fenceLen = n
				out = append(out, line) // preserve fence opener unchanged
				continue
			}
			// Outside a fence: apply tag removal to this line.
			replaced := removeInline.ReplaceAllStringFunc(line, func(match string) string {
				// Preserve the leading non-tag character if present.
				if len(match) > 0 && !strings.HasPrefix(match, "#") {
					return string(match[0])
				}
				return ""
			})
			out = append(out, replaced)
		} else {
			// Inside a fence: check for closing delimiter.
			if removeTagFenceCloser(trimmed, fenceChar, fenceLen) {
				inFence = false
			}
			out = append(out, line) // preserve fence content unchanged
		}
	}

	newBody := strings.Join(out, "\n")

	var assembled string
	if hasFM && mapping != nil {
		marshaledFM, marshalErr := yaml.Marshal(mapping)
		if marshalErr != nil {
			return fmt.Errorf("remove_tag: marshal: %w", marshalErr)
		}
		assembled = "---\n" + string(marshaledFM) + "---\n" + newBody
	} else {
		assembled = newBody
	}

	if writeErr := os.WriteFile(absPath, []byte(assembled), 0644); writeErr != nil {
		return &PathError{Op: "remove_tag", Path: path, Err: writeErr}
	}

	return nil
}

// removeTagFenceOpener reports whether line opens a fenced code block.
// It returns the fence character ('`' or '~') and the run length (≥3) on match,
// or 0, 0 when the line is not a fence opener.
// The fence must start at column 0 and consist of 3 or more identical characters.
func removeTagFenceOpener(line string) (byte, int) {
	for _, ch := range []byte{'`', '~'} {
		if len(line) >= 3 && line[0] == ch && line[1] == ch && line[2] == ch {
			n := 3
			for n < len(line) && line[n] == ch {
				n++
			}
			return ch, n
		}
	}
	return 0, 0
}

// removeTagFenceCloser reports whether line closes a fence that was opened with
// fenceChar and run length fenceLen. A closer requires at least fenceLen
// consecutive fenceChar characters, optionally followed by spaces only.
func removeTagFenceCloser(line string, fenceChar byte, fenceLen int) bool {
	if len(line) < fenceLen {
		return false
	}
	i := 0
	for i < len(line) && line[i] == fenceChar {
		i++
	}
	if i < fenceLen {
		return false
	}
	for ; i < len(line); i++ {
		if line[i] != ' ' {
			return false
		}
	}
	return true
}

// AggregateTags walks the entire vault and returns a map from tag name to the
// number of notes it appears in (frontmatter + inline, deduplicated per note).
//
// The walk honours the vault's PathFilter (IsIgnored + IsAllowedExtension) and
// skips unreadable files silently.  The method is read-only and does not lock
// the service mutex.
func (s *Service) AggregateTags(ctx context.Context) (map[string]int, error) {
	counts := make(map[string]int)

	err := s.WalkNotes(ctx, func(rel, abs string) error {
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			return nil // skip unreadable files silently
		}

		content := string(data)
		rawFM, body, hasFM := SplitFrontmatter(content)

		// Collect per-note tags (deduplicated within the note).
		noteTags := make(map[string]struct{})

		if hasFM {
			fm, parseErr := ParseFrontmatter(rawFM)
			if parseErr == nil {
				for _, t := range ExtractFrontmatterTags(fm) {
					noteTags[t] = struct{}{}
				}
			}
		}

		for _, t := range ExtractInlineTags(body) {
			noteTags[t] = struct{}{}
		}

		for t := range noteTags {
			counts[t]++
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("aggregate_tags: walk: %w", err)
	}

	return counts, nil
}

// TagCount is a tag name with its occurrence count.
type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TopTagsByCount returns up to limit entries from counts, sorted by Count
// descending and Name ascending as tiebreaker. If limit <= 0 or limit
// exceeds len(counts), all entries are returned.
func TopTagsByCount(counts map[string]int, limit int) []TagCount {
	out := make([]TagCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, TagCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}
