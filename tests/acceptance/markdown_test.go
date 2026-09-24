package acceptance_test

import (
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
)

func TestMarkdown_Acceptance(t *testing.T) {
	t.Run("[AC-1] StripCodeFences leaves text with no fences or inline code unchanged", func(t *testing.T) {
		input := "Hello world\nThis is plain text."
		got := markdown.StripCodeFences(input)
		if got != input {
			t.Errorf("StripCodeFences(%q) = %q, want unchanged", input, got)
		}
	})

	t.Run("[AC-2] StripCodeFences removes triple-backtick fenced blocks, any language tag, content included", func(t *testing.T) {
		tests := []struct {
			input   string
			mustNot string
		}{
			{"Before\n```go\nfunc main() {}\n```\nAfter", "func main"},
			{"```typescript\nconst x = 1;\n```", "const x"},
		}
		for _, tc := range tests {
			got := markdown.StripCodeFences(tc.input)
			if strings.Contains(got, tc.mustNot) {
				t.Errorf("StripCodeFences(%q) = %q; must not contain %q", tc.input, got, tc.mustNot)
			}
		}
	})

	t.Run("[AC-3] StripCodeFences removes triple-tilde fenced blocks the same as backtick fences", func(t *testing.T) {
		got := markdown.StripCodeFences("Start\n~~~python\nprint('hello')\n~~~\nEnd")
		if strings.Contains(got, "print('hello')") {
			t.Errorf("tilde-fenced content leaked into output: %q", got)
		}
		if !strings.Contains(got, "Start") || !strings.Contains(got, "End") {
			t.Errorf("prose around the fence was stripped: %q", got)
		}
	})

	t.Run("[AC-4] a closing fence must match the opener's character and be at least as long", func(t *testing.T) {
		got := markdown.StripCodeFences("````\nblock\n````")
		if strings.Contains(got, "block") {
			t.Errorf("StripCodeFences(%q) = %q; fenced content leaked", "````\\nblock\\n````", got)
		}
	})

	t.Run("[AC-5] CRLF line endings do not prevent a fence from closing", func(t *testing.T) {
		got := markdown.StripCodeFences("Before\r\n```\r\ncode here\r\n```\r\nAfter")
		if strings.Contains(got, "code here") {
			t.Errorf("CRLF fence did not close, content leaked: %q", got)
		}
		if !strings.Contains(got, "After") {
			t.Errorf("expected content after the closed fence to survive: %q", got)
		}
	})

	t.Run("[AC-6] an unclosed fence consumes the rest of the document", func(t *testing.T) {
		got := markdown.StripCodeFences("Before\n```\nunclosed code")
		if strings.Contains(got, "unclosed code") {
			t.Errorf("unclosed fence content leaked: %q", got)
		}
		if !strings.Contains(got, "Before") {
			t.Errorf("prose before the unclosed fence was stripped: %q", got)
		}
	})

	t.Run("[AC-7] single-backtick inline code spans are replaced with a single space; multi-backtick runs pass through", func(t *testing.T) {
		got := markdown.StripCodeFences("Call `foo()` now.")
		if got != "Call   now." {
			t.Errorf("StripCodeFences(%q) = %q, want %q", "Call `foo()` now.", got, "Call   now.")
		}

		preserved := markdown.StripCodeFences("Use ``df`` command.")
		if preserved != "Use ``df`` command." {
			t.Errorf("double-backtick span should pass through unchanged, got %q", preserved)
		}
	})

	t.Run("[AC-8] backticks inside a fenced block are not treated as inline code spans", func(t *testing.T) {
		got := markdown.StripCodeFences("Text\n~~~\nsome `code` here\n~~~\nMore")
		if strings.Contains(got, "some `code` here") {
			t.Errorf("fence content with backticks leaked as if inline-stripped: %q", got)
		}
	})

	t.Run("[AC-9] Tokenize lowercases and extracts letter/digit runs of length >= 2, dropping punctuation", func(t *testing.T) {
		got := markdown.Tokenize("Machine Learning in Go, v1 v12!")
		want := []string{"machine", "learning", "in", "go", "v1", "v12"}
		if len(got) != len(want) {
			t.Fatalf("Tokenize(...) = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("Tokenize(...)[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
			}
		}
	})

	t.Run("[AC-10] Tokenize returns nil when no token reaches the 2-character minimum", func(t *testing.T) {
		if got := markdown.Tokenize(""); got != nil {
			t.Errorf("Tokenize(\"\") = %v, want nil", got)
		}
		if got := markdown.Tokenize("a b c"); got != nil {
			t.Errorf("Tokenize(\"a b c\") = %v, want nil (all tokens below minimum length)", got)
		}
	})

	t.Run("[AC-11] StripCodeFences composed with Tokenize keeps code identifiers out of the token stream while preserving prose", func(t *testing.T) {
		input := "# My Note\n\nSome prose text.\n\n```go\nfunc secretFunc() {}\n```\n\nMore prose here."
		tokens := markdown.Tokenize(markdown.StripCodeFences(input))

		for _, tok := range tokens {
			if tok == "secretfunc" {
				t.Errorf("code identifier leaked into tokens: %v", tokens)
			}
		}
		found := false
		for _, tok := range tokens {
			if tok == "prose" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected 'prose' in tokens, got %v", tokens)
		}
	})
}
