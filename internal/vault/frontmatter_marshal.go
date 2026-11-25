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

// detectSequenceIndent returns the most common indentation of a block
// sequence item ("- ") relative to the line of the key that owns it, or
// defaultFrontmatterIndent when there is none. Zero-offset sequences
// ("key:\n- a") cannot be produced by yaml.v3 and are ignored.
func detectSequenceIndent(src string) int {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	counts := map[int]int{}
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
			continue
		}
		itemIndent := len(line) - len(trimmed)
		// The owning key is the nearest previous line indented less than
		// this item (a sibling "- " at the same indent is skipped).
		for j := i - 1; j >= 0; j-- {
			prev := lines[j]
			pt := strings.TrimLeft(prev, " ")
			if pt == "" || strings.HasPrefix(pt, "#") {
				continue
			}
			pi := len(prev) - len(pt)
			if pi < itemIndent {
				if !strings.HasPrefix(pt, "- ") && strings.HasSuffix(strings.TrimRight(pt, " "), ":") {
					counts[itemIndent-pi]++
				}
				break
			}
		}
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
