package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestService_WriteMove_SymlinkedDirMissingSubdir(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		target  func(vaultDir, outside string) string
		wantErr error
	}{
		{"ignored target", func(v, _ string) string { return filepath.Join(v, ".obsidian") }, vault.ErrPathRestricted},
		{"outside vault", func(_, o string) string { return o }, vault.ErrSymlinkEscape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vaultDir := t.TempDir()
			target := tc.target(vaultDir, t.TempDir())
			require.NoError(t, os.MkdirAll(target, 0o755))
			if err := os.Symlink(target, filepath.Join(vaultDir, "link")); err != nil {
				t.Skipf("symlinks not supported: %v", err)
			}
			svc := vault.New(vaultDir, vault.NewPathFilter(defaultIgnore, defaultExts))

			err := svc.WriteNote(ctx, "link/sub/new.md", "x", vault.WriteModeOverwrite)
			require.ErrorIs(t, err, tc.wantErr)
			assert.NoDirExists(t, filepath.Join(target, "sub"))

			require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "n.md"), []byte("n"), 0o644))
			_, err = svc.MoveNote(ctx, "n.md", "link/sub2/n.md", "n.md", false, false)
			require.ErrorIs(t, err, tc.wantErr)
			assert.NoDirExists(t, filepath.Join(target, "sub2"))
			assert.FileExists(t, filepath.Join(vaultDir, "n.md"))
		})
	}
}

func TestService_RenameTag_CRLF(t *testing.T) {
	ctx := context.Background()

	cases := []struct{ name, in, want string }{
		{"inline-only hit", "---\r\ntitle: x\r\n---\r\nbody #old\r\n", "---\r\ntitle: x\r\n---\r\nbody #new\r\n"},
		{"frontmatter hit", "---\r\ntags:\r\n- old\r\n---\r\nbody\r\n", "---\r\ntags:\r\n  - new\r\n---\r\nbody\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte(tc.in), 0o644))
			svc := vault.New(dir, vault.NewPathFilter(defaultIgnore, defaultExts))

			_, err := svc.RenameTag(ctx, "old", "new")
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(dir, "c.md"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
