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

// ----------------------------------------------------------------------------
// WalkNotes
// ----------------------------------------------------------------------------

func TestService_WalkNotes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("nil filter visits all files without panic", func(t *testing.T) {
		t.Parallel()

		// Create a vault with two files and no filter.
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.md"), []byte("b"), 0644))

		svc := vault.New(dir, nil)

		var visited []string
		err := svc.WalkNotes(ctx, func(rel, abs string) error {
			visited = append(visited, rel)
			return nil
		})
		require.NoError(t, err)
		assert.Contains(t, visited, "a.md")
		assert.Contains(t, visited, "sub/b.md")
	})

	t.Run("IsIgnored on directory skips the whole subtree", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ignoredDir := filepath.Join(dir, "node_modules")
		require.NoError(t, os.MkdirAll(ignoredDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(ignoredDir, "pkg.md"), []byte("x"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.md"), []byte("y"), 0644))

		filter := vault.NewPathFilter([]string{"node_modules"}, []string{".md"})
		svc := vault.New(dir, filter)

		var visited []string
		err := svc.WalkNotes(ctx, func(rel, abs string) error {
			visited = append(visited, rel)
			return nil
		})
		require.NoError(t, err)
		assert.Contains(t, visited, "keep.md")
		assert.NotContains(t, visited, "node_modules/pkg.md")
	})

	t.Run("IsIgnored on file skips that file only", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.md"), []byte("keep"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0644))

		// Use ".DS_Store" as both an ignored pattern and not an allowed extension,
		// so we can observe ignore-by-name rather than extension filtering.
		filter := vault.NewPathFilter([]string{".DS_Store"}, []string{".md"})
		svc := vault.New(dir, filter)

		var visited []string
		err := svc.WalkNotes(ctx, func(rel, abs string) error {
			visited = append(visited, rel)
			return nil
		})
		require.NoError(t, err)
		assert.Contains(t, visited, "keep.md")
		assert.NotContains(t, visited, ".DS_Store")
	})

	t.Run("fn returning SkipAll stops walk with nil error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0644))

		filter := vault.NewPathFilter(defaultIgnore, defaultExts)
		svc := vault.New(dir, filter)

		var visitCount int
		err := svc.WalkNotes(ctx, func(rel, abs string) error {
			visitCount++
			return filepath.SkipAll
		})
		require.NoError(t, err)
		// Only the first file triggers SkipAll; the walk stops there.
		assert.Equal(t, 1, visitCount)
	})

	t.Run("non-existent root returns non-nil error", func(t *testing.T) {
		t.Parallel()

		svc := vault.New("/does/not/exist/at/all", nil)

		err := svc.WalkNotes(ctx, func(rel, abs string) error {
			return nil
		})
		require.Error(t, err)
	})

	t.Run("cancelled context aborts walk", func(t *testing.T) {
		t.Parallel()

		// Use the shared fixture vault so there are enough files to walk.
		svc := newService(testVaultRoot)

		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before walk starts

		err := svc.WalkNotes(cancelCtx, func(rel, abs string) error {
			return nil
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
