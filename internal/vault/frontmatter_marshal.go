package vault

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultFrontmatterIndent is used when the source frontmatter has no block
// sequence to infer an indent from.
const defaultFrontmatterIndent = 2

// marshalFrontmatter encodes mapping like yaml.Marshal, but with an indent
// inferred from srcFrontmatter so untouched block sequences keep the width
// they had on disk (yaml.Marshal's fixed 4-space default would re-indent
// "list:\n  - q" to "list:\n    - q"). Flow sequences are unaffected.
func marshalFrontmatter(mapping *yaml.Node, srcFrontmatter string) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(detectSequenceIndent(srcFrontmatter))
	if err := enc.Encode(mapping); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// maxIndentSamples caps how many block-sequence items are sampled; the most
// common width is stable long before this, and the cap bounds the work on
// hostile input.
const maxIndentSamples = 10000

// indentEntry is a previous non-blank, non-comment line on the owner stack.
type indentEntry struct {
	indent int
	owner  bool // a "key:" line (not a "- " item) that can own a sequence
}

// stripYAMLComment removes a trailing " #..." comment from a line. A '#'
// only starts a comment when preceded by whitespace and outside quotes.
func stripYAMLComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

// detectSequenceIndent returns the most common indentation of a block
// sequence item ("- ") relative to the line of the key that owns it, or
// defaultFrontmatterIndent when there is none. Zero-offset sequences
// ("key:\n- a") cannot be produced by yaml.v3 and are ignored. It runs in
// linear time using a stack of enclosing lines with strictly increasing
// indent; the owner of a line is the nearest previous line indented less.
func detectSequenceIndent(src string) int {
	counts := map[int]int{}
	var stack []indentEntry
	samples := 0
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(trimmed)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		isItem := strings.HasPrefix(trimmed, "- ") || trimmed == "-"
		if isItem && len(stack) > 0 && stack[len(stack)-1].owner {
			counts[indent-stack[len(stack)-1].indent]++
			samples++
			if samples >= maxIndentSamples {
				break
			}
		}
		owner := !isItem && strings.HasSuffix(strings.TrimRight(stripYAMLComment(trimmed), " \t"), ":")
		stack = append(stack, indentEntry{indent: indent, owner: owner})
	}
	best, bestN := defaultFrontmatterIndent, 0
	for w, n := range counts {
		if w < 2 || w > 8 {
			continue
		}
		if n > bestN || (n == bestN && w < best) {
			best, bestN = w, n
		}
	}
	return best
}
