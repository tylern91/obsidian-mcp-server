package search

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripCodeFences(t *testing.T) {
	// Direct assertion for empty string (table guard skips wantExact == "").
	require.Empty(t, StripCodeFences(""))

	tests := []struct {
		name  string
		input string
		// We verify that the stripped tokens no longer contain code content.
		// For exact output we check specific cases.
		wantSubstring    string // must appear in output
		wantNotSubstring string // must NOT appear in output
		wantExact        string // exact match when non-empty
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
			name:      "longer opener requires longer closer",
			input:     "````\nblock\n````",
			wantSubstring:    " ",
			wantNotSubstring: "block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripCodeFences(tt.input)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("StripCodeFences(%q)\ngot:  %q\nwant: %q", tt.input, got, tt.wantExact)
			}
			if tt.wantSubstring != "" && !strings.Contains(got, tt.wantSubstring) {
				t.Errorf("StripCodeFences(%q) = %q; want it to contain %q", tt.input, got, tt.wantSubstring)
			}
			if tt.wantNotSubstring != "" && strings.Contains(got, tt.wantNotSubstring) {
				t.Errorf("StripCodeFences(%q) = %q; want it NOT to contain %q", tt.input, got, tt.wantNotSubstring)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize(%q)\ngot:  %v\nwant: %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize(%q)\ngot:  %v\nwant: %v", tt.input, got, tt.want)
					break
				}
			}
		})
	}
}

// TestStripCodeFencesTokenizeIntegration verifies that tokens from code blocks
// do not appear in the tokenized output of stripped content.
func TestStripCodeFencesTokenizeIntegration(t *testing.T) {
	input := "# My Note\n\nSome prose text.\n\n```go\nfunc secretFunc() {}\n```\n\nMore prose here."

	stripped := StripCodeFences(input)
	tokens := Tokenize(stripped)

	// "secretfunc" must NOT be a token.
	for _, tok := range tokens {
		if tok == "secretfunc" || tok == "secretFunc" {
			t.Errorf("code token %q leaked into output; tokens = %v", tok, tokens)
		}
	}

	// "prose" must appear.
	found := false
	for _, tok := range tokens {
		if tok == "prose" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'prose' in tokens; got %v", tokens)
	}
}
