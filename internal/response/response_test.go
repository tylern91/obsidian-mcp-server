package response_test

import (
	"math"
	"os"
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

// TestCountTokens_NoNetworkFetch asserts the encoder loads from the embedded
// cl100k_base rank table rather than tiktoken-go's default loader, which
// fetches over HTTP and caches the result to TIKTOKEN_CACHE_DIR on first use.
// If CountTokens ever regressed to the network loader, this directory would
// gain a cache file; with the embedded loader it must stay empty.
func TestCountTokens_NoNetworkFetch(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", cacheDir)

	got := response.CountTokens("the quick brown fox")
	if got <= 0 {
		t.Fatalf("CountTokens returned %d, want > 0", got)
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", cacheDir, err)
	}
	if len(entries) != 0 {
		t.Errorf("TIKTOKEN_CACHE_DIR gained %d entr(y/ies); CountTokens should never touch the network loader's cache", len(entries))
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
