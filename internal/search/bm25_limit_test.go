package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// Token counting is expensive; it must run only for results that survive the limit.
func TestSearchBM25_CountsTokensOnlyForReturnedResults(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 30; i++ {
		name := filepath.Join(root, fmt.Sprintf("n%02d.md", i))
		require.NoError(t, os.WriteFile(name, []byte("common term here\n"), 0o644))
	}
	var calls int32
	restore := search.SetCountTokens(func(s string) int {
		atomic.AddInt32(&calls, 1)
		return len(s)
	})
	defer restore()

	pf := vault.NewPathFilter([]string{".obsidian"}, []string{".md"})
	svc := search.New(vault.New(root, pf))
	res, err := svc.SearchBM25(context.Background(), search.BM25Options{
		Query: "common", Limit: 5, SearchContent: true,
	})
	require.NoError(t, err)
	require.Len(t, res, 5)
	require.EqualValues(t, 5, atomic.LoadInt32(&calls))
}
