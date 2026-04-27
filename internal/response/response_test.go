package response_test

import (
	"math"
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/response"
)

func TestCountTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{
			name:    "empty string",
			text:    "",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "single word",
			text:    "hello",
			wantMin: 1,
			wantMax: 3,
		},
		{
			name:    "sentence returns positive count",
			text:    "The quick brown fox jumps over the lazy dog.",
			wantMin: 8,
			wantMax: 20,
		},
		{
			name:    "longer text scales up",
			text:    strings.Repeat("word ", 100),
			wantMin: 50,
			wantMax: 300,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := response.CountTokens(tc.text)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("CountTokens(%q) = %d, want [%d, %d]", tc.text, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestFormatJSON_Compact(t *testing.T) {
	t.Parallel()

	data := map[string]any{"key": "value", "num": 42}
	got, err := response.FormatJSON(data, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "\n") || strings.Contains(got, "  ") {
		t.Errorf("compact JSON should not contain newlines or indentation, got: %s", got)
	}
	if !strings.Contains(got, `"num"`) {
		t.Errorf("JSON missing expected key, got: %s", got)
	}
}

func TestFormatJSON_Pretty(t *testing.T) {
	t.Parallel()

	data := map[string]any{"key": "value"}
	got, err := response.FormatJSON(data, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("pretty JSON should contain newlines, got: %s", got)
	}
	if !strings.Contains(got, "  ") {
		t.Errorf("pretty JSON should contain indentation, got: %s", got)
	}
}

func TestFormatJSON_ErrorOnUnmarshalable(t *testing.T) {
	t.Parallel()

	// math.Inf produces a float that is not valid JSON
	_, err := response.FormatJSON(math.Inf(1), false)
	if err == nil {
		t.Error("expected error for non-JSON-encodable value (math.Inf), got nil")
	}
}

// ── Truncate ─────────────────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		s        string
		maxRunes int
		wantS    string
		wantCut  bool
	}{
		{
			name:     "empty string no cut",
			s:        "",
			maxRunes: 10,
			wantS:    "",
			wantCut:  false,
		},
		{
			name:     "exactly maxRunes no cut",
			s:        "hello",
			maxRunes: 5,
			wantS:    "hello",
			wantCut:  false,
		},
		{
			name:     "one over cuts",
			s:        "hello!",
			maxRunes: 5,
			wantS:    "hello",
			wantCut:  true,
		},
		{
			name:     "ASCII well under no cut",
			s:        "hi",
			maxRunes: 10,
			wantS:    "hi",
			wantCut:  false,
		},
		{
			name:     "multibyte CJK cut on rune boundary",
			s:        "你好世界",
			maxRunes: 2,
			wantS:    "你好",
			wantCut:  true,
		},
		{
			name:     "emoji single rune cut",
			s:        "🎉abc",
			maxRunes: 1,
			wantS:    "🎉",
			wantCut:  true,
		},
		{
			name:     "emoji no cut",
			s:        "🎉",
			maxRunes: 5,
			wantS:    "🎉",
			wantCut:  false,
		},
		{
			name:     "CRLF pair not split — cut before \\r",
			s:        "ab\r\ncd",
			maxRunes: 3, // runes: a b \r \n c d — cut at 3 would separate \r from \n
			wantS:    "ab",
			wantCut:  true,
		},
		{
			name:     "CRLF pair not split — both included when cut after \\n",
			s:        "ab\r\ncd",
			maxRunes: 4, // runes: a b \r \n → include both \r and \n
			wantS:    "ab\r\n",
			wantCut:  true,
		},
		{
			name:     "CRLF no split needed",
			s:        "ab\r\ncd",
			maxRunes: 10,
			wantS:    "ab\r\ncd",
			wantCut:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotS, gotCut := response.Truncate(tc.s, tc.maxRunes)
			if gotS != tc.wantS || gotCut != tc.wantCut {
				t.Errorf("Truncate(%q, %d) = (%q, %v), want (%q, %v)",
					tc.s, tc.maxRunes, gotS, gotCut, tc.wantS, tc.wantCut)
			}
		})
	}
}

// ── HeadRunes ────────────────────────────────────────────────────────────────

func TestHeadRunes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{
			name: "empty string",
			s:    "",
			n:    10,
			want: "",
		},
		{
			name: "n larger than length",
			s:    "hello",
			n:    20,
			want: "hello",
		},
		{
			name: "exactly n runes",
			s:    "hello",
			n:    5,
			want: "hello",
		},
		{
			name: "ASCII truncated",
			s:    "hello world",
			n:    5,
			want: "hello",
		},
		{
			name: "CJK multibyte truncated",
			s:    "你好世界",
			n:    3,
			want: "你好世",
		},
		{
			name: "emoji single rune",
			s:    "🎉🎊🎈",
			n:    2,
			want: "🎉🎊",
		},
		{
			name: "n zero returns empty",
			s:    "hello",
			n:    0,
			want: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := response.HeadRunes(tc.s, tc.n)
			if got != tc.want {
				t.Errorf("HeadRunes(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		})
	}
}
