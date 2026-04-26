package vault_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// ----------------------------------------------------------------------------
// SplitFrontmatter
// ----------------------------------------------------------------------------

func TestSplitFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantRaw   string
		wantBody  string
		wantHasFM bool
	}{
		{
			name:      "empty string",
			content:   "",
			wantRaw:   "",
			wantBody:  "",
			wantHasFM: false,
		},
		{
			name:      "content with no frontmatter",
			content:   "# Hello\n\nSome content.\n",
			wantRaw:   "",
			wantBody:  "# Hello\n\nSome content.\n",
			wantHasFM: false,
		},
		{
			name:      "valid frontmatter",
			content:   "---\ntitle: Test\n---\n\nBody here.\n",
			wantRaw:   "title: Test\n",
			wantBody:  "\nBody here.\n",
			wantHasFM: true,
		},
		{
			name:      "frontmatter with no closing delimiter",
			content:   "---\ntitle: Test\nno closing here\n",
			wantRaw:   "",
			wantBody:  "---\ntitle: Test\nno closing here\n",
			wantHasFM: false,
		},
		{
			name:      "frontmatter with empty body",
			content:   "---\ntitle: Test\n---\n",
			wantRaw:   "title: Test\n",
			wantBody:  "",
			wantHasFM: true,
		},
		{
			name:      "starts with --- but no newline after opening",
			content:   "---title: Test---",
			wantRaw:   "",
			wantBody:  "---title: Test---",
			wantHasFM: false,
		},
		{
			name:      "multiline frontmatter",
			content:   "---\ntitle: Note\ntags:\n  - go\n  - test\n---\n\n# Heading\n",
			wantRaw:   "title: Note\ntags:\n  - go\n  - test\n",
			wantBody:  "\n# Heading\n",
			wantHasFM: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, body, hasFM := vault.SplitFrontmatter(tc.content)
			assert.Equal(t, tc.wantRaw, raw, "raw YAML")
			assert.Equal(t, tc.wantBody, body, "body text")
			assert.Equal(t, tc.wantHasFM, hasFM, "hasFM flag")
		})
	}
}

// ----------------------------------------------------------------------------
// ParseFrontmatter
// ----------------------------------------------------------------------------

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("empty raw returns empty map", func(t *testing.T) {
		t.Parallel()
		fm, err := vault.ParseFrontmatter("")
		require.NoError(t, err)
		assert.Empty(t, fm)
	})

	t.Run("whitespace-only raw returns empty map", func(t *testing.T) {
		t.Parallel()
		fm, err := vault.ParseFrontmatter("   \n  ")
		require.NoError(t, err)
		assert.Empty(t, fm)
	})

	t.Run("valid YAML scalar values", func(t *testing.T) {
		t.Parallel()
		raw := "title: My Note\npriority: high\ncount: 42\nenabled: true\n"
		fm, err := vault.ParseFrontmatter(raw)
		require.NoError(t, err)
		assert.Equal(t, "My Note", fm["title"])
		assert.Equal(t, "high", fm["priority"])
		assert.Equal(t, 42, fm["count"])
		assert.Equal(t, true, fm["enabled"])
	})

	t.Run("valid YAML with sequence", func(t *testing.T) {
		t.Parallel()
		raw := "tags:\n  - go\n  - test\n"
		fm, err := vault.ParseFrontmatter(raw)
		require.NoError(t, err)
		tags, ok := fm["tags"].([]any)
		require.True(t, ok, "tags should be []any")
		assert.Contains(t, tags, "go")
		assert.Contains(t, tags, "test")
	})

	t.Run("invalid YAML returns ErrInvalidFrontmatter", func(t *testing.T) {
		t.Parallel()
		raw := "title: [\nbroken yaml\n"
		_, err := vault.ParseFrontmatter(raw)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrInvalidFrontmatter), "error should wrap ErrInvalidFrontmatter, got: %v", err)
	})
}

// ----------------------------------------------------------------------------
// Service.GetFrontmatter
// ----------------------------------------------------------------------------

func TestService_GetFrontmatter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService(testVaultRoot)

	t.Run("note with frontmatter returns parsed map", func(t *testing.T) {
		t.Parallel()
		fm, body, err := svc.GetFrontmatter(ctx, "Notes/with-fm.md")
		require.NoError(t, err)

		assert.Equal(t, "Note With Frontmatter", fm["title"])
		assert.Equal(t, "high", fm["priority"])
		assert.Equal(t, 42, fm["count"])
		assert.Equal(t, true, fm["enabled"])

		tags, ok := fm["tags"].([]any)
		require.True(t, ok, "tags should be []any")
		assert.Contains(t, tags, "research")
		assert.Contains(t, tags, "ideas")

		assert.Contains(t, body, "# Note With Frontmatter")
		assert.NotContains(t, body, "---")
	})

	t.Run("note without frontmatter returns empty map and full content", func(t *testing.T) {
		t.Parallel()
		fm, body, err := svc.GetFrontmatter(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.Empty(t, fm)
		assert.Contains(t, body, "# Simple Note")
	})

	t.Run("non-existent note returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		_, _, err := svc.GetFrontmatter(ctx, "Notes/does-not-exist.md")
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})
}

// ----------------------------------------------------------------------------
// Service.UpdateFrontmatter
// ----------------------------------------------------------------------------

func TestService_UpdateFrontmatter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	copyFixture := func(t *testing.T) (tempRoot, notePath string) {
		t.Helper()
		dir := t.TempDir()
		notesDir := filepath.Join(dir, "Notes")
		require.NoError(t, os.MkdirAll(notesDir, 0755))

		src := filepath.Join(testVaultRoot, "Notes", "with-fm.md")
		dst := filepath.Join(notesDir, "with-fm.md")

		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, data, 0644))

		return dir, "Notes/with-fm.md"
	}

	t.Run("update existing key preserves other keys", func(t *testing.T) {
		t.Parallel()
		root, notePath := copyFixture(t)
		svc := newService(root)

		require.NoError(t, svc.UpdateFrontmatter(ctx, notePath, map[string]any{"priority": "low"}, nil))

		fm, _, err := svc.GetFrontmatter(ctx, notePath)
		require.NoError(t, err)
		assert.Equal(t, "low", fm["priority"])
		assert.Equal(t, "Note With Frontmatter", fm["title"])
		assert.Equal(t, 42, fm["count"])
		assert.Equal(t, true, fm["enabled"])
	})

	t.Run("add new key", func(t *testing.T) {
		t.Parallel()
		root, notePath := copyFixture(t)
		svc := newService(root)

		require.NoError(t, svc.UpdateFrontmatter(ctx, notePath, map[string]any{"author": "Tyler"}, nil))

		fm, _, err := svc.GetFrontmatter(ctx, notePath)
		require.NoError(t, err)
		assert.Equal(t, "Tyler", fm["author"])
		assert.Equal(t, "Note With Frontmatter", fm["title"])
	})

	t.Run("remove existing key", func(t *testing.T) {
		t.Parallel()
		root, notePath := copyFixture(t)
		svc := newService(root)

		require.NoError(t, svc.UpdateFrontmatter(ctx, notePath, nil, []string{"priority"}))

		fm, _, err := svc.GetFrontmatter(ctx, notePath)
		require.NoError(t, err)
		_, hasPriority := fm["priority"]
		assert.False(t, hasPriority)
		assert.Equal(t, "Note With Frontmatter", fm["title"])
	})

	t.Run("remove non-existent key is a no-op", func(t *testing.T) {
		t.Parallel()
		root, notePath := copyFixture(t)
		svc := newService(root)

		require.NoError(t, svc.UpdateFrontmatter(ctx, notePath, nil, []string{"nonexistent_key"}))

		fm, _, err := svc.GetFrontmatter(ctx, notePath)
		require.NoError(t, err)
		assert.Equal(t, "Note With Frontmatter", fm["title"])
	})

	t.Run("round-trip preserves body content", func(t *testing.T) {
		t.Parallel()
		root, notePath := copyFixture(t)
		svc := newService(root)

		require.NoError(t, svc.UpdateFrontmatter(ctx, notePath, map[string]any{"priority": "low"}, nil))

		fm, body, err := svc.GetFrontmatter(ctx, notePath)
		require.NoError(t, err)
		assert.Equal(t, "low", fm["priority"])
		assert.Equal(t, "Note With Frontmatter", fm["title"])
		assert.Equal(t, 42, fm["count"])
		assert.Equal(t, true, fm["enabled"])
		assert.Contains(t, body, "# Note With Frontmatter")
	})

	t.Run("non-existent file returns ErrNotFound", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc := newService(dir)

		err := svc.UpdateFrontmatter(ctx, "Notes/missing.md", map[string]any{"k": "v"}, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrNotFound))
	})

	t.Run("symlink escape returns ErrSymlinkEscape", func(t *testing.T) {
		t.Parallel()
		vaultDir := t.TempDir()
		outsideDir := t.TempDir()

		outsideFile := filepath.Join(outsideDir, "secret.md")
		require.NoError(t, os.WriteFile(outsideFile, []byte("---\ntitle: secret\n---\nbody\n"), 0644))

		symlinkPath := filepath.Join(vaultDir, "escape.md")
		if err := os.Symlink(outsideFile, symlinkPath); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}

		svc := newService(vaultDir)

		err := svc.UpdateFrontmatter(ctx, "escape.md", map[string]any{"k": "v"}, nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, vault.ErrSymlinkEscape), "got: %v", err)
	})

	t.Run("note with no frontmatter gets frontmatter prepended", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		notesDir := filepath.Join(dir, "Notes")
		require.NoError(t, os.MkdirAll(notesDir, 0755))

		src := filepath.Join(testVaultRoot, "Notes", "simple.md")
		dst := filepath.Join(notesDir, "simple.md")
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, data, 0644))

		svc := newService(dir)

		require.NoError(t, svc.UpdateFrontmatter(ctx, "Notes/simple.md", map[string]any{"title": "New Title"}, nil))

		fm, body, err := svc.GetFrontmatter(ctx, "Notes/simple.md")
		require.NoError(t, err)
		assert.Equal(t, "New Title", fm["title"])
		assert.Contains(t, body, "# Simple Note")
	})
}
