package vault

import (
	"net/url"
	"path/filepath"
	"strings"
)

// anchorChange describes a heading rename for the link rewriter.
type anchorChange struct {
	oldText, newText string
}

// slugOf is the lowercase, hyphen-joined form Markdown-style links use for a
// heading anchor ("My Heading" -> "my-heading").
func slugOf(heading string) string {
	return strings.ToLower(strings.Join(strings.Fields(heading), "-"))
}

// rewriteAnchorLine rewrites links on one line whose target resolves to the
// note being edited (per mv.targets) and whose heading anchor names the old
// heading. A wikilink anchor "#A#Old" (heading path) has its last component
// replaced; block references ("#^id") are never touched. A link whose bare
// target is shared by another note is reported as ambiguous and left alone,
// exactly like move_note.
func rewriteAnchorLine(line string, mv *moveCtx) (string, []LinkRewrite) {
	type match struct {
		start, end int
		rewrite    LinkRewrite
	}
	var matches []match
	oldText, newText := mv.anchor.oldText, mv.anchor.newText

	for _, m := range wikilinkComponentRegex.FindAllStringSubmatchIndex(line, -1) {
		if m[6] == -1 || m[6] == m[7] {
			continue
		}
		target := line[m[4]:m[5]]
		targetSlash := filepath.ToSlash(target)
		if !mv.targets[targetSlash] && !mv.targets[trimExt(targetSlash, mv.oldExt)] {
			continue
		}
		anchor := line[m[6]:m[7]] // "#A#Old"
		parts := strings.Split(anchor[1:], "#")
		last := parts[len(parts)-1]
		if strings.HasPrefix(last, "^") || !strings.EqualFold(strings.TrimSpace(last), oldText) {
			continue
		}
		oldLink := line[m[0]:m[1]]
		if !strings.Contains(target, "/") && len(mv.index[trimExt(targetSlash, mv.oldExt)]) > 1 {
			matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldLink, Ambiguous: true}})
			continue
		}
		parts[len(parts)-1] = newText
		alias := ""
		if m[8] != -1 {
			alias = line[m[8]:m[9]]
		}
		newLink := line[m[2]:m[3]] + "[[" + target + "#" + strings.Join(parts, "#") + alias + "]]"
		matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldLink, NewText: newLink}})
	}

	for _, m := range markdownLinkComponentRegex.FindAllStringSubmatchIndex(line, -1) {
		if m[6] == -1 || m[6] == m[7] {
			continue
		}
		rawPath := line[m[4]:m[5]]
		decoded := decodeMarkdownPath(rawPath)
		if !mv.targets[decoded] && !mv.targets[trimExt(decoded, mv.oldExt)] {
			continue
		}
		rawFrag := line[m[6]:m[7]][1:]
		frag := decodeMarkdownPath(rawFrag)
		var replacement string
		switch {
		case strings.EqualFold(frag, oldText):
			replacement = newText
		case frag == slugOf(oldText):
			replacement = slugOf(newText)
		default:
			continue
		}
		if strings.Contains(rawFrag, "%") {
			replacement = url.PathEscape(replacement)
		}
		oldLink := line[m[0]:m[1]]
		if !strings.Contains(decoded, "/") && len(mv.index[trimExt(decoded, mv.oldExt)]) > 1 {
			matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldLink, Ambiguous: true}})
			continue
		}
		newLink := line[m[2]:m[3]] + rawPath + "#" + replacement + line[m[8]:m[9]]
		matches = append(matches, match{m[0], m[1], LinkRewrite{OldText: oldLink, NewText: newLink}})
	}

	if len(matches) == 0 {
		return line, nil
	}
	// Wikilink and Markdown matches never overlap; order by position so the
	// right-to-left splice below stays valid and results read in line order.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
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
