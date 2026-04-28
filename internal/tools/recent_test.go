package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/tools"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// recentResponse is the shape returned by get_recent_changes.
type recentResponse struct {
	Notes []struct {
		Path    string  `json:"path"`
		ModTime string  `json:"modTime"`
		HeadOf  *string `json:"headOf,omitempty"`
	} `json:"notes"`
	Count int `json:"count"`
}

func TestGetRecentChangesHandler_Basic(t *testing.T) {
	deps := testDeps(t)
	handler := tools.RecentChangesHandler(deps)

	result, err := handler(context.Background(), makeRequest())
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %v", result.Content)

	text := extractText(t, result)
	var resp recentResponse
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Greater(t, resp.Count, 0, "fixture vault has notes")
	assert.Len(t, resp.Notes, resp.Count)

	// All modTimes must be valid RFC3339.
	for _, n := range resp.Notes {
		_, parseErr := time.Parse(time.RFC3339, n.ModTime)
		assert.NoError(t, parseErr, "modTime %q must be RFC3339", n.ModTime)
	}
}

func TestGetRecentChangesHandler_Limit(t *testing.T) {
	deps := testDeps(t)
	handler := tools.RecentChangesHandler(deps)

	result, err := handler(context.Background(), makeRequest("limit", "2"))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %v", result.Content)

	text := extractText(t, result)
	var resp recentResponse
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	// fixture vault has >2 notes, so count should be exactly 2
	assert.Equal(t, 2, resp.Count)
	assert.Len(t, resp.Notes, 2)
}

func TestGetRecentChangesHandler_Since_FutureDate(t *testing.T) {
	deps := testDeps(t)
	handler := tools.RecentChangesHandler(deps)

	result, err := handler(context.Background(), makeRequest("since", "2099-01-01"))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %v", result.Content)

	text := extractText(t, result)
	var resp recentResponse
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, 0, resp.Count, "no notes should be newer than 2099-01-01")
}

func TestGetRecentChangesHandler_Since_InvalidDate(t *testing.T) {
	deps := testDeps(t)
	handler := tools.RecentChangesHandler(deps)

	result, err := handler(context.Background(), makeRequest("since", "not-a-date"))
	require.NoError(t, err)
	assert.True(t, result.IsError, "invalid since date should return an error")
}

func TestGetRecentChangesHandler_Ordering(t *testing.T) {
	// Build a mutable vault so we can set specific mtimes.
	vaultRoot := t.TempDir()
	require.NoError(t, copyDirForTools("../../testdata/vault", vaultRoot))

	filter := vault.NewPathFilter(
		[]string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"},
		[]string{".md", ".markdown", ".txt", ".canvas"},
	)
	deps := tools.Deps{
		Vault:       vault.New(vaultRoot, filter),
		PrettyPrint: false,
	}
	handler := tools.RecentChangesHandler(deps)

	// First call: discover 3 note paths.
	result, err := handler(context.Background(), makeRequest("limit", "3"))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := extractText(t, result)
	var initial recentResponse
	require.NoError(t, json.Unmarshal([]byte(text), &initial))
	require.GreaterOrEqual(t, len(initial.Notes), 3, "need at least 3 notes for ordering test")

	paths := []string{initial.Notes[0].Path, initial.Notes[1].Path, initial.Notes[2].Path}

	// Assign specific times: paths[0]=oldest, paths[1]=middle, paths[2]=newest.
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2023, 3, 10, 8, 0, 0, 0, time.UTC)

	for i, p := range paths {
		abs := filepath.Join(vaultRoot, filepath.FromSlash(p))
		mt := []time.Time{t1, t2, t3}[i]
		require.NoError(t, os.Chtimes(abs, mt, mt))
	}

	// Second call after setting times — expect newest first.
	result2, err := handler(context.Background(), makeRequest("limit", "3"))
	require.NoError(t, err)
	require.False(t, result2.IsError)

	text2 := extractText(t, result2)
	var resp recentResponse
	require.NoError(t, json.Unmarshal([]byte(text2), &resp))
	require.GreaterOrEqual(t, len(resp.Notes), 3)

	// Verify overall descending ordering of returned notes.
	for i := 1; i < len(resp.Notes); i++ {
		prev, _ := time.Parse(time.RFC3339, resp.Notes[i-1].ModTime)
		curr, _ := time.Parse(time.RFC3339, resp.Notes[i].ModTime)
		assert.False(t, curr.After(prev), "notes[%d] modTime should not be after notes[%d]", i, i-1)
	}

	// The note with the newest mtime (paths[2]) should appear before paths[1] and paths[0].
	pathToIdx := map[string]int{}
	for idx, n := range resp.Notes {
		pathToIdx[n.Path] = idx
	}
	if _, ok := pathToIdx[paths[2]]; ok {
		if _, ok2 := pathToIdx[paths[1]]; ok2 {
			assert.Less(t, pathToIdx[paths[2]], pathToIdx[paths[1]],
				"newest note (%s) should appear before middle note (%s)", paths[2], paths[1])
		}
		if _, ok2 := pathToIdx[paths[0]]; ok2 {
			assert.Less(t, pathToIdx[paths[2]], pathToIdx[paths[0]],
				"newest note (%s) should appear before oldest note (%s)", paths[2], paths[0])
		}
	}
}

func TestGetRecentChangesHandler_SummaryTrue(t *testing.T) {
	deps := testDeps(t)
	handler := tools.RecentChangesHandler(deps)

	result, err := handler(context.Background(), makeRequest("limit", "2", "summary", "true"))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %v", result.Content)

	text := extractText(t, result)
	var resp recentResponse
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Greater(t, resp.Count, 0)

	for _, n := range resp.Notes {
		assert.NotNil(t, n.HeadOf, "headOf should be present when summary=true")
		if n.HeadOf != nil {
			assert.NotEmpty(t, *n.HeadOf)
		}
	}
}
