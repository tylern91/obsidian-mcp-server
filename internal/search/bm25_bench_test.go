package search_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// BenchmarkSearchBM25 measures end-to-end BM25 search performance over the
// fixture vault, including both corpus-stat pass and scoring pass.
func BenchmarkSearchBM25(b *testing.B) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatal("runtime.Caller failed")
	}
	// file is .../internal/search/bm25_bench_test.go
	// testdata/vault is two directories up from internal/search
	vaultRoot := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "vault")

	pf := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	v := vault.New(vaultRoot, pf)
	svc := search.New(v)

	opts := search.BM25Options{
		Query:             "daily notes",
		SearchContent:     true,
		SearchFrontmatter: true,
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := svc.SearchBM25(ctx, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
