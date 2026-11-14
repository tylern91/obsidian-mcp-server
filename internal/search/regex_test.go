package search_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func TestSearchRegex_LongLineDoesNotFailVault(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte(strings.Repeat("x", 70000)+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("needle\n"), 0o644))

	svc := search.New(vault.New(root, nil))
	results, err := svc.SearchRegex(context.Background(), search.RegexOptions{Pattern: "needle", Scope: "content"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "b.md", results[0].Path)
}
