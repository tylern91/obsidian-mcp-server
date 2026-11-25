package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

func totalFixture(t *testing.T, n int) *search.Service {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < n; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("n%02d.md", i)), []byte("common term here\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.md"), []byte("nothing relevant\n"), 0o644))
	return search.New(vault.New(root, vault.NewPathFilter(nil, []string{".md"})))
}

func TestSearchBM25Total_CountsBeforeLimit(t *testing.T) {
	svc := totalFixture(t, 30)
	res, total, err := svc.SearchBM25Total(context.Background(), search.BM25Options{Query: "common", Limit: 5, SearchContent: true})
	require.NoError(t, err)
	require.Len(t, res, 5)
	require.Equal(t, 30, total)

	res, total, err = svc.SearchBM25Total(context.Background(), search.BM25Options{Query: "common", Limit: 100, SearchContent: true})
	require.NoError(t, err)
	require.Equal(t, len(res), total)
}

func TestSearchRegexTotal_StopsOneBeyondLimit(t *testing.T) {
	svc := totalFixture(t, 30)
	res, total, err := svc.SearchRegexTotal(context.Background(), search.RegexOptions{Pattern: "common", Limit: 5})
	require.NoError(t, err)
	require.Len(t, res, 5)
	// The walk stops at the first match beyond the limit: a lower bound.
	require.Equal(t, 6, total)

	res, total, err = svc.SearchRegexTotal(context.Background(), search.RegexOptions{Pattern: "common", Limit: 100})
	require.NoError(t, err)
	require.Equal(t, len(res), total)

	// An exact fit is not truncated.
	res, total, err = svc.SearchRegexTotal(context.Background(), search.RegexOptions{Pattern: "common", Limit: 30})
	require.NoError(t, err)
	require.Len(t, res, 30)
	require.Equal(t, 30, total)

	// MaxResults caps the effective limit; total is limit+1 when truncated.
	res, total, err = svc.SearchRegexTotal(context.Background(), search.RegexOptions{Pattern: "common", Limit: 100, MaxResults: 3})
	require.NoError(t, err)
	require.Len(t, res, 3)
	require.Equal(t, 4, total)
}
