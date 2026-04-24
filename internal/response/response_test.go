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
