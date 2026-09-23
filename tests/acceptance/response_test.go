package acceptance_test

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

// requestWithArgs builds a CallToolRequest carrying the given arguments.
func requestWithArgs(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func TestResponse_Acceptance(t *testing.T) {
	t.Run("[AC-1] FormatJSON with prettyPrint=false produces compact JSON with no indentation", func(t *testing.T) {
		got, err := response.FormatJSON(map[string]any{"key": "value", "num": 42}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(got, "\n") || strings.Contains(got, "  ") {
			t.Errorf("compact JSON should not contain newlines or indentation, got: %s", got)
		}
		if !strings.Contains(got, `"num"`) {
			t.Errorf("JSON missing expected key, got: %s", got)
		}
	})

	t.Run("[AC-2] FormatJSON with prettyPrint=true produces 2-space indented JSON", func(t *testing.T) {
		got, err := response.FormatJSON(map[string]any{"key": "value"}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(got, "\n") {
			t.Errorf("pretty JSON should contain newlines, got: %s", got)
		}
		if !strings.Contains(got, "  ") {
			t.Errorf("pretty JSON should contain indentation, got: %s", got)
		}
	})

	t.Run("[AC-3] FormatJSON returns an error, not a panic, for values that cannot be marshaled", func(t *testing.T) {
		_, err := response.FormatJSON(math.Inf(1), false)
		if err == nil {
			t.Error("expected error for non-JSON-encodable value (math.Inf), got nil")
		}
	})

	t.Run("[AC-4] HeadRunes truncates by rune count, never splitting a multi-byte rune", func(t *testing.T) {
		tests := []struct {
			name string
			s    string
			n    int
			want string
		}{
			{"empty string", "", 10, ""},
			{"n larger than length", "hello", 20, "hello"},
			{"exactly n runes", "hello", 5, "hello"},
			{"ASCII truncated", "hello world", 5, "hello"},
			{"CJK multibyte truncated", "你好世界", 3, "你好世"},
			{"emoji single rune", "🎉🎊🎈", 2, "🎉🎊"},
			{"n zero returns empty", "hello", 0, ""},
		}
		for _, tc := range tests {
			got := response.HeadRunes(tc.s, tc.n)
			if got != tc.want {
				t.Errorf("HeadRunes(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		}
	})

	t.Run("[AC-5] CountTokens counts via the embedded cl100k_base table, never the network loader", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("TIKTOKEN_CACHE_DIR", cacheDir)

		got := response.CountTokens("The quick brown fox jumps over the lazy dog.")
		if got < 8 || got > 20 {
			t.Errorf("CountTokens = %d, want in [8, 20]", got)
		}

		entries, err := os.ReadDir(cacheDir)
		if err != nil {
			t.Fatalf("ReadDir(%q): %v", cacheDir, err)
		}
		if len(entries) != 0 {
			t.Errorf("TIKTOKEN_CACHE_DIR gained %d entr(y/ies); CountTokens must never touch the network loader's cache", len(entries))
		}
	})

	t.Run("[AC-6] ToolResult honors the request's prettyPrint argument over the handler default", func(t *testing.T) {
		req := requestWithArgs(map[string]any{"prettyPrint": true})
		result, err := response.ToolResult(req, false, map[string]any{"key": "value"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := toolResultText(t, result)
		if !strings.Contains(text, "\n") {
			t.Errorf("expected pretty-printed JSON honoring the request argument, got: %s", text)
		}
	})

	t.Run("[AC-7] ToolResult falls back to the handler default when the request omits prettyPrint", func(t *testing.T) {
		req := requestWithArgs(nil)
		result, err := response.ToolResult(req, true, map[string]any{"key": "value"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := toolResultText(t, result)
		if !strings.Contains(text, "\n") {
			t.Errorf("expected default prettyPrint=true to apply, got: %s", text)
		}
	})

	t.Run("[AC-8] ToolResult returns a tool error result, not a Go error, when marshaling fails", func(t *testing.T) {
		req := requestWithArgs(nil)
		result, err := response.ToolResult(req, false, math.Inf(1))
		if err != nil {
			t.Fatalf("ToolResult must never surface a Go error (it would become a protocol error): %v", err)
		}
		if result == nil || !result.IsError {
			t.Errorf("expected an error tool result, got: %+v", result)
		}
	})

	t.Run("[AC-9] JSONResourceContents wraps successfully-marshaled JSON with the given URI and MIME type", func(t *testing.T) {
		contents := response.JSONResourceContents("note://a.md", "application/json", map[string]any{"key": "value"}, false)
		if len(contents) != 1 {
			t.Fatalf("expected exactly one resource content, got %d", len(contents))
		}
		trc, ok := contents[0].(mcp.TextResourceContents)
		if !ok {
			t.Fatalf("expected mcp.TextResourceContents, got %T", contents[0])
		}
		if trc.URI != "note://a.md" || trc.MIMEType != "application/json" {
			t.Errorf("got URI=%q MIMEType=%q, want URI=%q MIMEType=%q", trc.URI, trc.MIMEType, "note://a.md", "application/json")
		}
		if !strings.Contains(trc.Text, `"key"`) {
			t.Errorf("resource text missing expected key, got: %s", trc.Text)
		}
	})

	t.Run("[AC-10] JSONResourceContents falls back to an error envelope when marshaling fails", func(t *testing.T) {
		contents := response.JSONResourceContents("note://a.md", "application/json", math.Inf(1), false)
		trc, ok := contents[0].(mcp.TextResourceContents)
		if !ok {
			t.Fatalf("expected mcp.TextResourceContents, got %T", contents[0])
		}
		if !strings.Contains(trc.Text, `"error"`) || !strings.Contains(trc.Text, `"uri"`) {
			t.Errorf("expected an {error,uri} envelope, got: %s", trc.Text)
		}
	})

	t.Run("[AC-11] ErrorResourceContents always returns a single-item error,uri JSON envelope", func(t *testing.T) {
		contents := response.ErrorResourceContents("note://a.md", "boom")
		if len(contents) != 1 {
			t.Fatalf("expected exactly one resource content, got %d", len(contents))
		}
		trc, ok := contents[0].(mcp.TextResourceContents)
		if !ok {
			t.Fatalf("expected mcp.TextResourceContents, got %T", contents[0])
		}
		if trc.MIMEType != "application/json" {
			t.Errorf("MIMEType = %q, want application/json", trc.MIMEType)
		}
		if !strings.Contains(trc.Text, `"error":"boom"`) || !strings.Contains(trc.Text, `"uri":"note://a.md"`) {
			t.Errorf("expected error/uri fields in envelope, got: %s", trc.Text)
		}
	})
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatalf("expected non-empty tool result content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected mcp.TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
