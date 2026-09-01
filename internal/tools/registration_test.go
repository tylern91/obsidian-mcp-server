package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func registrationTestDeps(t *testing.T) Deps {
	t.Helper()
	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	return Deps{
		Vault: vault.New(t.TempDir(), filter),
	}
}

func TestAllSpecs(t *testing.T) {
	wantMutating := map[string]bool{
		"read_note":                 false,
		"write_note":                true,
		"list_directory":            false,
		"get_frontmatter":           false,
		"update_frontmatter":        true,
		"manage_tags":               true,
		"list_all_tags":             false,
		"get_backlinks":             false,
		"patch_note":                true,
		"delete_note":               true,
		"move_note":                 true,
		"search_notes":              false,
		"search_regex":              false,
		"read_multiple_notes":       false,
		"get_notes_info":            false,
		"get_vault_stats":           false,
		"get_recent_changes":        false,
		"get_periodic_note":         true,
		"get_recent_periodic_notes": false,
		"audit_notes":               false,
		"get_note_outline":          false,
		"read_note_lines":           false,
	}

	deps := registrationTestDeps(t)
	specs := allSpecs(deps)

	if len(specs) != len(wantMutating) {
		t.Fatalf("allSpecs returned %d specs, want %d", len(specs), len(wantMutating))
	}

	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		name := spec.Tool.Name
		if name == "" {
			t.Fatal("spec has empty tool name")
		}
		if seen[name] {
			t.Fatalf("duplicate tool name %q", name)
		}
		seen[name] = true

		if spec.Handler == nil {
			t.Errorf("%s: nil handler", name)
		}

		want, ok := wantMutating[name]
		if !ok {
			t.Errorf("%s: unexpected tool name, not in expectation table", name)
			continue
		}
		if spec.Mutating != want {
			t.Errorf("%s: Mutating = %v, want %v", name, spec.Mutating, want)
		}
	}

	for name := range wantMutating {
		if !seen[name] {
			t.Errorf("expected tool %q missing from allSpecs", name)
		}
	}
}

func TestRegisterAll_ReadOnly_OmitsMutatingTools(t *testing.T) {
	deps := registrationTestDeps(t)
	deps.ReadOnly = true

	s := server.NewMCPServer("test", "0.0.0")
	RegisterAll(s, deps)

	registered := s.ListTools()
	for _, spec := range allSpecs(deps) {
		_, ok := registered[spec.Tool.Name]
		if spec.Mutating && ok {
			t.Errorf("%s: mutating tool must not be registered in read-only mode", spec.Tool.Name)
		}
		if !spec.Mutating && !ok {
			t.Errorf("%s: read-only tool should still be registered in read-only mode", spec.Tool.Name)
		}
	}
}

func TestRegisterAll_NotReadOnly_RegistersEverything(t *testing.T) {
	deps := registrationTestDeps(t)

	s := server.NewMCPServer("test", "0.0.0")
	RegisterAll(s, deps)

	registered := s.ListTools()
	for _, spec := range allSpecs(deps) {
		if _, ok := registered[spec.Tool.Name]; !ok {
			t.Errorf("%s: expected to be registered when not read-only", spec.Tool.Name)
		}
	}
}

func TestReadOnlyGuard_RejectsWhenReadOnly(t *testing.T) {
	called := false
	inner := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("should not run"), nil
	}

	guarded := readOnlyGuard(true, inner)
	result, err := guarded(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected an error result when read-only")
	}
	if called {
		t.Error("inner handler must not run when read-only")
	}
}

func TestReadOnlyGuard_PassesThroughWhenNotReadOnly(t *testing.T) {
	called := false
	inner := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("ran"), nil
	}

	guarded := readOnlyGuard(false, inner)
	if _, err := guarded(context.Background(), mcp.CallToolRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("inner handler should run when not read-only")
	}
}

func TestNewToolSpecDerivesMutatingFromReadOnlyHint(t *testing.T) {
	deps := registrationTestDeps(t)

	for _, spec := range allSpecs(deps) {
		hint := spec.Tool.Annotations.ReadOnlyHint
		if hint == nil {
			if !spec.Mutating {
				t.Errorf("%s: nil ReadOnlyHint should default to Mutating=true", spec.Tool.Name)
			}
			continue
		}
		wantMutating := !*hint
		if spec.Mutating != wantMutating {
			t.Errorf("%s: Mutating=%v disagrees with ReadOnlyHint=%v", spec.Tool.Name, spec.Mutating, *hint)
		}
	}
}
