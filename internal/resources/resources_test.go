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

func (f *flakyStatsVault) Generation() uint64 { return 0 }

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

// Writes, moves and deletes of a nested note change no mtime on the vault
// root, so the cached stats and tags must be invalidated by the service itself.
func TestVaultResources_NestedNoteMutationsInvalidateCache(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filter := vault.NewPathFilter([]string{".obsidian", ".trash"}, []string{".md"})
	svc := vault.New(root, filter)
	deps := resources.Deps{Vault: svc}
	stats := resources.VaultStatsHandler(deps)
	tags := resources.VaultTagsHandler(deps)

	read := func(h func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error)) string {
		t.Helper()
		got, err := h(ctx, mcp.ReadResourceRequest{})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return got[0].(mcp.TextResourceContents).Text
	}
	mustChange := func(step string, h func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error), mutate func()) {
		t.Helper()
		before := read(h)
		mutate()
		if after := read(h); after == before {
			t.Errorf("%s: cached resource was not invalidated: %s", step, after)
		}
	}

	if err := svc.WriteNote(ctx, "folder/sub/note.md", "# one\n", vault.WriteModeOverwrite); err != nil {
		t.Fatal(err)
	}
	read(stats)
	read(tags)

	mustChange("edit stats", stats, func() {
		if err := svc.WriteNote(ctx, "folder/sub/note.md", "# one\n\nmore body text\n", vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
	})
	mustChange("edit tags", tags, func() {
		if err := svc.WriteNote(ctx, "folder/sub/note.md", "# one\n\n#nested-tag\n", vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
	})
	mustChange("move tags", tags, func() {
		if _, err := svc.MoveNote(ctx, "folder/sub/note.md", "other/note.md", "folder/sub/note.md", false, false); err != nil {
			t.Fatal(err)
		}
		if err := svc.WriteNote(ctx, "other/second.md", "#second-tag\n", vault.WriteModeOverwrite); err != nil {
			t.Fatal(err)
		}
	})
	mustChange("delete stats", stats, func() {
		if err := svc.DeleteNote(ctx, "other/second.md", "other/second.md", true); err != nil {
			t.Fatal(err)
		}
	})
}
