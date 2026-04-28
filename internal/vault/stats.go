package vault

import (
	"context"
	"fmt"
	"os"
	"time"
)

// NoteTimestamp identifies a note by relative path and a single timestamp.
// Used by VaultStats to track the oldest and newest notes in a vault.
type NoteTimestamp struct {
	Path    string
	ModTime time.Time
}

// VaultStats holds aggregate metrics produced by a single vault walk.
//
// TagCounts is the raw map[tag → notes containing that tag]; callers can call
// TopTagsByCount to produce ranked leaderboards. TotalTokens is 0 unless
// opts.TokenCounter was provided and opts.IncludeTokens was true.
type VaultStats struct {
	NoteCount   int
	TotalBytes  int64
	TotalLinks  int
	TotalTokens int
	Oldest      *NoteTimestamp
	Newest      *NoteTimestamp
	TagCounts   map[string]int
}

// VaultStatsOpts configures the VaultStats walk.
//
// TokenCounter is provided by the caller (typically wired to
// internal/response.CountTokens) so that the vault package does not depend on
// the response package.
type VaultStatsOpts struct {
	IncludeTokens bool
	TokenCounter  func(text string) int
}

// VaultStats performs a single walk over the vault and returns aggregate metrics.
// It honours the vault's path filter and silently skips files that fail to open
// or stat.
//
// Tag counting uses per-note deduplication (same semantics as AggregateTags):
// a tag that appears twice in one note counts as one.
func (s *Service) VaultStats(ctx context.Context, opts VaultStatsOpts) (*VaultStats, error) {
	stats := &VaultStats{TagCounts: make(map[string]int)}

	err := s.WalkNotes(ctx, func(rel, abs string) error {
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return nil
		}
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			return nil
		}
		content := string(data)

		stats.NoteCount++
		stats.TotalBytes += info.Size()

		// Per-note dedup: count each tag once per note.
		noteTags := make(map[string]struct{})
		for _, t := range MergeNoteTags(data) {
			noteTags[t] = struct{}{}
		}
		for t := range noteTags {
			stats.TagCounts[t]++
		}

		stats.TotalLinks += len(ExtractLinks(content))

		if opts.IncludeTokens && opts.TokenCounter != nil {
			stats.TotalTokens += opts.TokenCounter(content)
		}

		mt := info.ModTime()
		if stats.Oldest == nil || mt.Before(stats.Oldest.ModTime) {
			stats.Oldest = &NoteTimestamp{Path: rel, ModTime: mt}
		}
		if stats.Newest == nil || mt.After(stats.Newest.ModTime) {
			stats.Newest = &NoteTimestamp{Path: rel, ModTime: mt}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("vault_stats: walk: %w", err)
	}
	return stats, nil
}
