package markdown

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
	var fenceChar byte  // '`' or '~'
	var fenceLength int // run length of the opener

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}

		// Trim trailing \r so CRLF files close fences correctly.
		trimmed := strings.TrimRight(line, "\r")

		if inFence {
			// Look for the closing fence: same fence character, at least fenceLength chars.
			if isFenceCloser(trimmed, fenceChar, fenceLength) {
				inFence = false
			}
			// Replace the entire line (opening, body, closing) with a space.
			out.WriteByte(' ')
			continue
		}

		// Check for opening fence at column 0.
		if ch, n, ok := fenceOpener(trimmed); ok {
			inFence = true
			fenceChar = ch
			fenceLength = n
			out.WriteByte(' ')
			continue
		}

		// Not in a fence — strip inline backtick spans.
		out.WriteString(stripInlineCode(line))
	}

	return out.String()
}

// fenceOpener reports whether line starts a fenced code block.
// It returns the fence character ('`' or '~'), the run length, and true on match.
// The fence must be at column 0 and consist of 3 or more identical characters.
func fenceOpener(line string) (byte, int, bool) {
	for _, ch := range []byte{'`', '~'} {
		if len(line) >= 3 && line[0] == ch && line[1] == ch && line[2] == ch {
			// Count the full run of the fence character.
			n := 3
			for n < len(line) && line[n] == ch {
				n++
			}
			// Remaining characters (after the run) must not contain the fence char
			// itself (CommonMark: backtick fences require no backtick in info string).
			// For simplicity: we accept any opener that starts with 3+ fence chars.
			return ch, n, true
		}
	}
	return 0, 0, false
}

// isFenceCloser reports whether line closes an open fence started with fenceChar
// and the original run length fenceLen. A closer requires at least fenceLen
// consecutive fenceChar characters, optionally followed by spaces only.
func isFenceCloser(line string, fenceChar byte, fenceLen int) bool {
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
	// Remainder must be empty or spaces only.
	for ; i < len(line); i++ {
		if line[i] != ' ' {
			return false
		}
	}
	return true
}

// stripInlineCode replaces single-backtick inline code spans with a space.
// Multi-backtick spans (`` `code` ``, etc.) are passed through unchanged.
func stripInlineCode(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}

	var out strings.Builder
	out.Grow(len(line))

	i := 0
	for i < len(line) {
		if line[i] == '`' {
			// Count the run of consecutive backticks at position i.
			runStart := i
			for i < len(line) && line[i] == '`' {
				i++
			}
			runLen := i - runStart

			if runLen > 1 {
				// Multi-backtick run: pass through as-is (no span matching).
				out.WriteString(line[runStart:i])
				continue
			}

			// runLen == 1: attempt to find the closing single backtick.
			j := i
			for j < len(line) && line[j] != '`' {
				j++
			}
			if j < len(line) {
				// Span found: replace entire span with a single space.
				out.WriteByte(' ')
				i = j + 1
			} else {
				// No closing backtick: pass the opening backtick through.
				out.WriteByte('`')
				// i is already past the backtick
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
