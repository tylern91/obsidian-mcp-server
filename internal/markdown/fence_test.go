package markdown

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// StripCodeFences
// ----------------------------------------------------------------------------

func TestStripCodeFences(t *testing.T) {
	t.Parallel()

	// Guard: empty string.
	require.Empty(t, StripCodeFences(""))

	tests := []struct {
		name             string
		input            string
		wantExact        string // checked when non-empty
		wantSubstring    string // must appear in output
		wantNotSubstring string // must NOT appear in output
	}{
		{
			name:      "no fences passthrough",
			input:     "Hello world\nThis is plain text.",
			wantExact: "Hello world\nThis is plain text.",
		},
		{
			name:             "triple-backtick block removed",
			input:            "Before\n```go\nfunc main() {}\n```\nAfter",
			wantSubstring:    "Before",
			wantNotSubstring: "func main",
		},
		{
			name:             "triple-tilde block removed",
			input:            "Start\n~~~python\nprint('hello')\n~~~\nEnd",
			wantSubstring:    "Start",
			wantNotSubstring: "print('hello')",
		},
		{
			name:             "inline backtick removed",
			input:            "Use `os.Exit(1)` to quit.",
			wantSubstring:    "Use",
			wantNotSubstring: "os.Exit",
		},
		{
			name:      "inline backtick replaced with space keeps surrounding text",
			input:     "Call `foo()` now.",
			wantExact: "Call   now.",
		},
		{
			name:             "backtick inside tilde fence not treated as inline code",
			input:            "Text\n~~~\nsome `code` here\n~~~\nMore",
			wantSubstring:    "Text",
			wantNotSubstring: "some `code` here",
		},
		{
			name:             "multiple inline backtick spans",
			input:            "Use `a` and `b` together.",
			wantSubstring:    "Use",
			wantNotSubstring: "a` and `b",
		},
		{
			name:             "fence with language tag",
			input:            "```typescript\nconst x = 1;\n```",
			wantNotSubstring: "const x",
		},
		{
			name:             "unclosed fence treats rest of document as code",
			input:            "Before\n```\nunclosed code",
			wantSubstring:    "Before",
			wantNotSubstring: "unclosed code",
		},
		{
			name:             "adjacent fences",
			input:            "```\nblock1\n```\nMiddle\n```\nblock2\n```",
			wantSubstring:    "Middle",
			wantNotSubstring: "block1",
		},
		{
			name:      "double-backtick inline span preserved",
			input:     "Use ``df`` command.",
			wantExact: "Use ``df`` command.",
		},
		{
			name:             "CRLF fence closer recognised",
			input:            "Before\r\n```\r\ncode here\r\n```\r\nAfter",
			wantSubstring:    "Before",
			wantNotSubstring: "code here",
		},
		{
			name:             "longer opener requires longer closer",
			input:            "````\nblock\n````",
			wantSubstring:    " ",
			wantNotSubstring: "block",
		},
		{
			name:             "tag inside backtick fence not visible",
			input:            "Prose\n```\n#fake_tag\n```\nMore prose",
			wantSubstring:    "Prose",
			wantNotSubstring: "#fake_tag",
		},
		{
			name:             "tag inside tilde fence not visible",
			input:            "Prose\n~~~\n#fake_tag\n~~~\nMore prose",
			wantSubstring:    "Prose",
			wantNotSubstring: "#fake_tag",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StripCodeFences(tc.input)

			if tc.wantExact != "" {
				assert.Equal(t, tc.wantExact, got)
			}
			if tc.wantSubstring != "" {
				assert.True(t, strings.Contains(got, tc.wantSubstring),
					"StripCodeFences(%q) = %q; want it to contain %q", tc.input, got, tc.wantSubstring)
			}
			if tc.wantNotSubstring != "" {
				assert.False(t, strings.Contains(got, tc.wantNotSubstring),
					"StripCodeFences(%q) = %q; want it NOT to contain %q", tc.input, got, tc.wantNotSubstring)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Tokenize
// ----------------------------------------------------------------------------

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "multi-word sentence",
			input: "Machine Learning in Go",
			want:  []string{"machine", "learning", "in", "go"},
		},
		{
			name:  "numbers included",
			input: "go1 version 21 release",
			want:  []string{"go1", "version", "21", "release"},
		},
		{
			name:  "punctuation stripped",
			input: "hello, world! foo-bar.",
			want:  []string{"hello", "world", "foo", "bar"},
		},
		{
			name:  "minimum length 2 enforced",
			input: "a ab abc",
			want:  []string{"ab", "abc"},
		},
		{
			name:  "unicode letters included",
			input: "Über café naïve",
			want:  []string{"über", "café", "naïve"},
		},
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "all short tokens filtered",
			input: "a b c",
			want:  nil,
		},
		{
			name:  "mixed case lowercased",
			input: "GoLang TypeScript RUST",
			want:  []string{"golang", "typescript", "rust"},
		},
		{
			name:  "digits only token",
			input: "version 42",
			want:  []string{"version", "42"},
		},
		{
			name:  "single char digits filtered",
			input: "v1 v12",
			want:  []string{"v1", "v12"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Tokenize(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ----------------------------------------------------------------------------
// StripCodeFences + Tokenize integration
// ----------------------------------------------------------------------------

func TestStripCodeFencesTokenizeIntegration(t *testing.T) {
	t.Parallel()

	input := "# My Note\n\nSome prose text.\n\n```go\nfunc secretFunc() {}\n```\n\nMore prose here."

	stripped := StripCodeFences(input)
	tokens := Tokenize(stripped)

	// "secretfunc" must NOT be a token.
	for _, tok := range tokens {
		assert.NotEqual(t, "secretfunc", tok, "code token %q leaked into output; tokens = %v", tok, tokens)
		assert.NotEqual(t, "secretFunc", tok, "code token %q leaked into output; tokens = %v", tok, tokens)
	}

	// "prose" must appear.
	assert.Contains(t, tokens, "prose", "expected 'prose' in tokens; got %v", tokens)
}
