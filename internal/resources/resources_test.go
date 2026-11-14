package resources_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"

	"github.com/tylern91/obsidian-mcp-server/internal/resources"
)

// TestPathFromURI covers pathFromURI edge cases (empty path suffix,
// non-matching prefix) that tests/acceptance/resources_test.go's ACs don't
// reach directly since every AC URI carries a matching prefix and a
// non-empty path.
func TestPathFromURI(t *testing.T) {
	cases := []struct {
		uri, prefix, want string
	}{
		{"obsidian://note/Notes/foo.md", "obsidian://note/", "Notes/foo.md"},
		{"obsidian://backlinks/Notes/bar.md", "obsidian://backlinks/", "Notes/bar.md"},
		{"obsidian://note/", "obsidian://note/", ""},
		{"obsidian://other/x", "obsidian://note/", ""},
	}
	for _, tc := range cases {
		got := resources.PathFromURI(tc.uri, tc.prefix)
		if got != tc.want {
			t.Errorf("PathFromURI(%q, %q) = %q, want %q", tc.uri, tc.prefix, got, tc.want)
		}
	}
}

type flakyStatsVault struct {
	resources.VaultService
	root  string
	calls int
}

func (f *flakyStatsVault) Root() string { return f.root }

func (f *flakyStatsVault) VaultStats(ctx context.Context, _ vault.VaultStatsOpts) (*vault.VaultStats, error) {
	f.calls++
	if f.calls == 1 {
		return nil, errors.New("transient walk failure")
	}
	return &vault.VaultStats{NoteCount: 3, TagCounts: map[string]int{}}, nil
}

// A transient failure must not be cached for the TTL.
func TestVaultStats_ErrorIsNotCached(t *testing.T) {
	deps := resources.Deps{Vault: &flakyStatsVault{root: t.TempDir()}}
	h := resources.VaultStatsHandler(deps)
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "obsidian://vault/stats"

	first, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if !strings.Contains(first[0].(mcp.TextResourceContents).Text, "error") {
		t.Fatalf("first read should carry an error payload, got %v", first)
	}

	second, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if text := second[0].(mcp.TextResourceContents).Text; strings.Contains(text, "error") {
		t.Errorf("second read served cached error: %s", text)
	}
}
