package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestService_WriteThroughRootSymlink_InternalStateRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	if err := os.Symlink(".", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	svc := newService(dir)

	err := svc.WriteNote(ctx, "link/.obsidian-mcp/trash/x.md", "x", vault.WriteModeOverwrite)
	require.Error(t, err)
	assert.ErrorIs(t, err, vault.ErrPathRestricted)
	assert.NoDirExists(t, filepath.Join(dir, ".obsidian-mcp"))
}

func TestService_DeleteAndPrune_SymlinkedTrashRootRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644))
	if err := os.Symlink(outside, filepath.Join(dir, ".obsidian-mcp")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	svc := newService(dir)

	err := svc.DeleteNote(ctx, "a.md", "a.md", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, vault.ErrSymlinkEscape)
	assert.FileExists(t, filepath.Join(dir, "a.md"))
	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "nothing may land outside the vault")

	require.NoError(t, os.MkdirAll(filepath.Join(outside, "trash", "old"), 0755))
	_, err = svc.PruneTrash(time.Now().Add(365*24*time.Hour), 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, vault.ErrSymlinkEscape)
	assert.DirExists(t, filepath.Join(outside, "trash", "old"))
}

func TestService_ReadNote_SymlinkToDisallowedExtensionRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.json"), []byte(`{"k":"v"}`), 0644))
	if err := os.Symlink("secret.json", filepath.Join(dir, "a.md")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	svc := newService(dir)

	_, err := svc.ReadNote(ctx, "a.md")
	require.Error(t, err)
	assert.ErrorIs(t, err, vault.ErrPathRestricted)
}

func TestService_TagMutations_PreserveCRLF(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name    string
		content string
		mutate  func(svc *vault.Service, path string) error
	}{
		{"add frontmatter tag", "---\r\ntitle: T\r\n---\r\n\r\nBody\r\n", func(s *vault.Service, p string) error {
			return s.AddTag(ctx, p, "new", "frontmatter")
		}},
		{"add inline tag", "---\r\ntitle: T\r\n---\r\n\r\nBody\r\n", func(s *vault.Service, p string) error {
			return s.AddTag(ctx, p, "new", "inline")
		}},
		{"remove frontmatter tag", "---\r\ntags:\r\n  - a\r\n  - b\r\n---\r\n\r\nBody\r\n", func(s *vault.Service, p string) error {
			return s.RemoveTag(ctx, p, "a")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, path := makeTempNote(t, tc.content)
			require.NoError(t, tc.mutate(newService(root), path))

			data, err := os.ReadFile(filepath.Join(root, path))
			require.NoError(t, err)
			got := string(data)
			assert.True(t, strings.HasPrefix(got, "---\r\n"), "frontmatter opener must stay CRLF: %q", got)
			delims := 0
			for _, line := range strings.Split(got, "\r\n") {
				if line == "---" {
					delims++
				}
			}
			assert.Equal(t, 2, delims, "exactly one frontmatter block expected: %q", got)
			assert.Equal(t, strings.Count(got, "\n"), strings.Count(got, "\r\n"), "no bare LF may remain: %q", got)
		})
	}
}
