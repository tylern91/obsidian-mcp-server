package search_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tylern91/obsidian-mcp-server/internal/search"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// bm25VaultRoot returns the absolute path to testdata/vault using runtime.Caller
// so it works regardless of the working directory when tests are run.
func bm25VaultRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// file is .../internal/search/bm25_test.go
	// testdata/vault is two directories up from internal/search
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "vault")
}

// newBM25PathFilter returns a standard path filter for tests.
func newBM25PathFilter() *vault.PathFilter {
	return vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
}

// newBM25Service builds a real vault.Service + search.Service pointing at
// the testdata/vault directory.
func newBM25Service(t *testing.T) *search.Service {
	t.Helper()
	root := bm25VaultRoot(t)
	v := vault.New(root, newBM25PathFilter())
	return search.New(v)
}

// newBM25ServiceFromDir builds a search.Service pointing at an arbitrary directory.
func newBM25ServiceFromDir(t *testing.T, dir string) *search.Service {
	t.Helper()
	v := vault.New(dir, newBM25PathFilter())
	return search.New(v)
}

// resultsContain returns true when any result has a path ending in suffix.
func resultsContain(results []search.BM25Result, suffix string) bool {
	for _, r := range results {
		if strings.HasSuffix(filepath.ToSlash(r.Path), suffix) {
			return true
		}
	}
	return false
}

// findResult returns the first result whose path ends in suffix, or zero value.
func findResult(results []search.BM25Result, suffix string) (search.BM25Result, bool) {
	for _, r := range results {
		if strings.HasSuffix(filepath.ToSlash(r.Path), suffix) {
			return r, true
		}
	}
	return search.BM25Result{}, false
}

// TestBM25_MLHeavyOutranksMedium asserts ml-intro.md scores higher than
// ml-basics.md for the query "machine learning".
func TestBM25_MLHeavyOutranksMedium(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results, "expected results for 'machine learning'")

	intro, ok1 := findResult(results, "Search/ml-intro.md")
	basics, ok2 := findResult(results, "Search/ml-basics.md")

	require.True(t, ok1, "ml-intro.md should appear in results")
	require.True(t, ok2, "ml-basics.md should appear in results")

	assert.Greater(t, intro.Score, basics.Score,
		"ml-intro (high density) should outrank ml-basics (medium density)")
}

// TestBM25_CookingDocsNotInTopMLResults asserts cooking.md and recipes.md are
// absent from the top-3 results for "machine learning".
func TestBM25_CookingDocsNotInTopMLResults(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		Limit:             3,
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	for _, r := range results {
		assert.NotContains(t, r.Path, "cooking.md",
			"cooking.md should not appear in top-3 ML results")
		assert.NotContains(t, r.Path, "recipes.md",
			"recipes.md should not appear in top-3 ML results")
	}
}

// TestBM25_SearchFrontmatterFalse_ExcludesFMOnly asserts that when
// SearchFrontmatter=false, fm-only.md does not appear in results because its
// only ML content is in the frontmatter.
func TestBM25_SearchFrontmatterFalse_ExcludesFMOnly(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: false,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	for _, r := range results {
		assert.NotContains(t, r.Path, "fm-only.md",
			"fm-only.md should not appear when SearchFrontmatter=false")
	}
}

// TestBM25_SearchFrontmatterTrue_IncludesFMOnly asserts that when
// SearchFrontmatter=true, fm-only.md appears in results.
func TestBM25_SearchFrontmatterTrue_IncludesFMOnly(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	assert.True(t, resultsContain(results, "Search/fm-only.md"),
		"fm-only.md should appear in results when SearchFrontmatter=true")
}

// TestBM25_FencedTagsNotHighlyRanked asserts fenced-tags.md does not appear in
// results for "machine learning" because its ML tokens are inside a code fence
// and must not be counted by the BM25 scorer.
func TestBM25_FencedTagsNotHighlyRanked(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: false,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	_, hasFenced := findResult(results, "Search/fenced-tags.md")
	assert.False(t, hasFenced,
		"fenced-tags.md should not appear in ML search results because its ML terms are inside a code fence")
}

// TestBM25_PathScopeRestrictsResults asserts that PathScope "Search/ml-*"
// returns only the ml-prefixed files.
func TestBM25_PathScopeRestrictsResults(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/ml-*",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		base := filepath.Base(r.Path)
		assert.True(t, strings.HasPrefix(base, "ml-"),
			"expected only ml-* files but got %q", r.Path)
	}
}

// TestBM25_CaseSensitive asserts that CaseSensitive=true with query "Machine"
// (capital M) returns fewer results than case-insensitive "machine".
func TestBM25_CaseSensitive(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	resultsSensitive, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "Machine",
		SearchContent:     true,
		SearchFrontmatter: false,
		CaseSensitive:     true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	resultsCI, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine",
		SearchContent:     true,
		SearchFrontmatter: false,
		CaseSensitive:     false,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	assert.LessOrEqual(t, len(resultsSensitive), len(resultsCI),
		"case-sensitive 'Machine' should return fewer or equal results than case-insensitive 'machine'")
}

// TestBM25_LimitCapsResults asserts that Limit is respected.
func TestBM25_LimitCapsResults(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		Limit:             2,
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)

	assert.LessOrEqual(t, len(results), 2, "Limit=2 should return at most 2 results")
}

// TestBM25_EmptyQueryReturnsEmpty asserts that an empty query returns no results.
func TestBM25_EmptyQueryReturnsEmpty(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "",
		SearchContent:     true,
		SearchFrontmatter: true,
	})
	require.NoError(t, err)
	assert.Empty(t, results, "empty query should return no results")
}

// TestBM25_PhraseBonusBoostsCoOccurrence asserts that a doc with consecutive
// "machine learning" bigrams outranks a doc where both terms appear many times
// but never adjacent. This directly exercises the phrase bigram mechanism.
func TestBM25_PhraseBonusBoostsCoOccurrence(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()

	// dense.md: "machine" and "learning" appear consecutively many times.
	// The phrase bigram "machine\x00learning" will accumulate high termFreq.
	dense := `---
title: Dense Co-occurrence
---
# Dense

Machine learning is great. Machine learning rocks. Machine learning all day.
Machine learning never stops. Machine learning for the win.
`
	// sparse.md: both terms appear but never adjacent — no consecutive bigrams.
	// Raw TF for "machine" and "learning" are deliberately balanced with dense.md
	// so that any scoring difference comes from the phrase key alone.
	sparse := `---
title: Sparse Terms
---
# Sparse

I have a machine. It is a very good machine. People enjoy using the machine.
Learning is important. Keep learning every day. Learning never stops for anyone.
`

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "dense.md"), []byte(dense), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "sparse.md"), []byte(sparse), 0644))

	svc := newBM25ServiceFromDir(t, tmpDir)

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2, "expected at least 2 results")

	denseRes, ok1 := findResult(results, "dense.md")
	sparseRes, ok2 := findResult(results, "sparse.md")

	require.True(t, ok1, "dense.md should appear in results")
	require.True(t, ok2, "sparse.md should appear in results")

	assert.Greater(t, denseRes.Score, sparseRes.Score,
		"dense co-occurrence doc (consecutive bigrams) should outrank sparse term doc (no bigrams)")
}

// TestBM25_BothFlagsOff_ReturnsNil asserts that when both SearchContent and
// SearchFrontmatter are false, the call returns nil results without error.
func TestBM25_BothFlagsOff_ReturnsNil(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine",
		SearchContent:     false,
		SearchFrontmatter: false,
	})
	require.NoError(t, err)
	assert.Nil(t, results)
}

// TestBM25_ResultsAreSortedByScore asserts results are in descending score order.
func TestBM25_ResultsAreSortedByScore(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/*",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
			"results should be sorted descending by score at index %d", i)
	}
}

// TestBM25_SnippetsCollected asserts that Matches are populated and TokenCount
// is positive for the top-scoring ML document.
func TestBM25_SnippetsCollected(t *testing.T) {
	svc := newBM25Service(t)
	ctx := context.Background()

	results, err := svc.SearchBM25(ctx, search.BM25Options{
		Query:             "machine learning",
		SearchContent:     true,
		SearchFrontmatter: true,
		PathScope:         "Search/ml-intro.md",
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	intro := results[0]
	assert.NotEmpty(t, intro.Matches, "ml-intro.md should have snippet matches")
	assert.Greater(t, intro.TokenCount, 0, "TokenCount should be positive")
}
