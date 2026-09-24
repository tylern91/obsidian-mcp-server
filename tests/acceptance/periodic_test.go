package acceptance_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
)

// newPeriodicService copies the fixture vault (which carries a real
// .obsidian/plugins/periodic-notes/data.json) into a fresh temp dir and
// returns a *periodic.Service pointed at it.
func newPeriodicService(t *testing.T) *periodic.Service {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir("../../testdata/vault", dir); err != nil {
		t.Fatalf("copy fixture vault: %v", err)
	}
	return periodic.New(dir)
}

// newPeriodicServiceWithConfig writes the given periodic-notes data.json
// content into a fresh temp dir and returns a *periodic.Service pointed at it.
func newPeriodicServiceWithConfig(t *testing.T, configJSON string) *periodic.Service {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".obsidian", "plugins", "periodic-notes")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "data.json"), []byte(configJSON), 0o644))
	return periodic.New(dir)
}

func fixedClock(year, month, day int) func() time.Time {
	return func() time.Time {
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}
}

func TestPeriodic_Acceptance(t *testing.T) {
	t.Run("[AC-1] LoadConfig reads granularity config from the vault's periodic-notes data.json", func(t *testing.T) {
		svc := newPeriodicService(t)
		cfg, err := svc.LoadConfig()
		require.NoError(t, err)

		daily, ok := cfg["daily"]
		require.True(t, ok, "expected 'daily' key in config")
		assert.True(t, daily.Enabled)
		assert.Equal(t, "YYYY-MM-DD", daily.Format)
		assert.Equal(t, "Daily Notes", daily.Folder)

		monthly, ok := cfg["monthly"]
		require.True(t, ok, "expected 'monthly' key in config")
		assert.False(t, monthly.Enabled)
	})

	t.Run("[AC-2] LoadConfig falls back to built-in defaults when the config file is missing", func(t *testing.T) {
		svc := periodic.New(t.TempDir())
		cfg, err := svc.LoadConfig()
		require.NoError(t, err)

		daily, ok := cfg["daily"]
		require.True(t, ok, "expected 'daily' default config")
		assert.True(t, daily.Enabled)
		assert.Equal(t, "YYYY-MM-DD", daily.Format)
		assert.Equal(t, "Daily Notes", daily.Folder)

		weekly, ok := cfg["weekly"]
		require.True(t, ok, "expected 'weekly' default config")
		assert.True(t, weekly.Enabled)
	})

	t.Run("[AC-3] Resolve computes today's daily note path from the configured format and folder", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("daily", 0)
		require.NoError(t, err)
		assert.Equal(t, "Daily Notes/2024-01-15.md", got)
	})

	t.Run("[AC-4] Resolve applies a negative offset to compute a past daily note path", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("daily", -1)
		require.NoError(t, err)
		assert.Equal(t, "Daily Notes/2024-01-14.md", got)
	})

	t.Run("[AC-5] Resolve computes the weekly note path using ISO week numbering", func(t *testing.T) {
		// 2024-01-15 is Monday of ISO week 3, year 2024.
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("weekly", 0)
		require.NoError(t, err)
		assert.Equal(t, "Weekly Notes/2024-W03.md", got)
	})

	t.Run("[AC-6] Resolve still computes a path for a granularity disabled in config", func(t *testing.T) {
		// Monthly is disabled in the fixture config, but Resolve should still work.
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("monthly", 0)
		require.NoError(t, err)
		assert.Equal(t, "Monthly Notes/2024-01.md", got)
	})

	t.Run("[AC-7] Resolve computes the quarterly note path", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("quarterly", 0)
		require.NoError(t, err)
		assert.Equal(t, "Quarterly Notes/2024-Q1.md", got)
	})

	t.Run("[AC-8] Resolve computes the yearly note path", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		got, err := svc.Resolve("yearly", 0)
		require.NoError(t, err)
		assert.Equal(t, "Yearly Notes/2024.md", got)
	})

	t.Run("[AC-9] Resolve returns an error for an unknown granularity", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		_, err := svc.Resolve("hourly", 0)
		require.Error(t, err)
	})

	t.Run("[AC-10] Resolve returns an error when the granularity's folder is not configured", func(t *testing.T) {
		svc := newPeriodicServiceWithConfig(t, `{"daily":{"enabled":true,"format":"YYYY-MM-DD","folder":"","template":""}}`).
			WithClock(fixedClock(2024, 1, 15))
		_, err := svc.Resolve("daily", 0)
		require.Error(t, err)
	})

	t.Run("[AC-11] RecentDates returns the N most recent dates in descending order", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		dates, err := svc.RecentDates("daily", 3)
		require.NoError(t, err)
		require.Len(t, dates, 3)

		want := []time.Time{
			time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC),
		}
		for i, d := range dates {
			assert.True(t, d.Equal(want[i]), "dates[%d]: got %v, want %v", i, d, want[i])
		}
	})

	t.Run("[AC-12] RecentDates returns an error for an unknown granularity", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		_, err := svc.RecentDates("hourly", 3)
		require.Error(t, err)
	})

	t.Run("[AC-13] RecentDates returns an error for a negative count", func(t *testing.T) {
		svc := newPeriodicService(t).WithClock(fixedClock(2024, 1, 15))
		_, err := svc.RecentDates("daily", -1)
		require.Error(t, err)
	})
}
