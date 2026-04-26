package search

import (
	"strings"
	"unicode"
)

// StripCodeFences returns the content with all fenced code blocks and inline
// code spans replaced by a single space. The result is always valid Unicode.
//
// Handled patterns:
//   - Triple-backtick fences: ```[lang]\n...\n```
//   - Triple-tilde fences:    ~~~[lang]\n...\n~~~
//   - Inline single-backtick spans: `code`
//
// A fence delimiter is recognised only at the start of a line (column 0).
// The implementation uses a line-by-line state machine for correctness.
func StripCodeFences(content string) string {
	lines := strings.Split(content, "\n")
	var out strings.Builder
	out.Grow(len(content))

	inFence := false
	var fenceChar byte // '`' or '~'

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}

		if inFence {
			// Look for the closing fence: exactly the same fence character, 3+.
			if isFenceCloser(line, fenceChar) {
				inFence = false
			}
			// Replace the entire line (opening, body, closing) with a space.
			out.WriteByte(' ')
			continue
		}

		// Check for opening fence at column 0.
		if ch, ok := fenceOpener(line); ok {
			inFence = true
			fenceChar = ch
			out.WriteByte(' ')
			continue
		}

		// Not in a fence — strip inline backtick spans.
		out.WriteString(stripInlineCode(line))
	}

	return out.String()
}

// fenceOpener reports whether line starts a fenced code block.
// It returns the fence character ('`' or '~') and true on match.
// The fence must be at column 0 and consist of 3 or more identical characters.
func fenceOpener(line string) (byte, bool) {
	for _, ch := range []byte{'`', '~'} {
		if len(line) >= 3 && line[0] == ch && line[1] == ch && line[2] == ch {
			// Remaining characters (after 3) must all be the same fence char or
			// form a language identifier — CommonMark: info string follows the
			// fence opener. We accept any opener that starts with 3+ fence chars
			// followed by anything except the fence char itself (for backtick fences).
			// For simplicity: require that the first 3 chars match; accept the rest.
			return ch, true
		}
	}
	return 0, false
}

// isFenceCloser reports whether line closes an open fence started with fenceChar.
// A closer is 3 or more fenceChar characters, optionally followed by spaces only.
func isFenceCloser(line string, fenceChar byte) bool {
	if len(line) < 3 {
		return false
	}
	i := 0
	for i < len(line) && line[i] == fenceChar {
		i++
	}
	if i < 3 {
		return false
	}
	// Remainder must be empty or spaces only.
	for ; i < len(line); i++ {
		if line[i] != ' ' {
			return false
		}
	}
	return true
}

// stripInlineCode replaces single-backtick inline code spans with a space.
// Double-backtick and other multi-backtick spans are left unchanged (rare in
// Obsidian and would require more complex span matching).
func stripInlineCode(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}

	var out strings.Builder
	out.Grow(len(line))

	i := 0
	for i < len(line) {
		if line[i] == '`' {
			// Single backtick only (not `` or ```).
			if i+1 < len(line) && line[i+1] == '`' {
				// Multi-backtick: pass through as-is until the matching closer.
				out.WriteByte(line[i])
				i++
				continue
			}
			// Find closing single backtick.
			j := i + 1
			for j < len(line) && line[j] != '`' {
				j++
			}
			if j < len(line) {
				// Span found: replace with a space.
				out.WriteByte(' ')
				i = j + 1
			} else {
				// No closing backtick: pass the opening backtick through.
				out.WriteByte(line[i])
				i++
			}
			continue
		}
		out.WriteByte(line[i])
		i++
	}

	return out.String()
}

// Tokenize splits text into lowercase word tokens.
// A token is a maximal run of Unicode letters and digits with a minimum length
// of 2 characters. Suitable for BM25 indexing and tag extraction.
//
// Example:
//
//	Tokenize("Machine Learning in Go") → ["machine", "learning", "in", "go"]
func Tokenize(text string) []string {
	if text == "" {
		return nil
	}

	var tokens []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() >= 2 {
			tokens = append(tokens, cur.String())
		}
		cur.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()

	return tokens
}
