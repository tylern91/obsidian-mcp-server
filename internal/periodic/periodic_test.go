package periodic_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tylern91/obsidian-mcp-server/internal/periodic"
)

func vaultRoot() string {
	return filepath.Join("..", "..", "testdata", "vault")
}

func fixedClock(year, month, day int) func() time.Time {
	return func() time.Time {
		return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	svc := periodic.New(vaultRoot())
	cfg, err := svc.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	daily, ok := cfg["daily"]
	if !ok {
		t.Fatal("expected 'daily' key in config")
	}
	if !daily.Enabled {
		t.Error("expected daily.Enabled=true")
	}
	if daily.Format != "YYYY-MM-DD" {
		t.Errorf("expected daily.Format=YYYY-MM-DD, got %q", daily.Format)
	}
	if daily.Folder != "Daily Notes" {
		t.Errorf("expected daily.Folder='Daily Notes', got %q", daily.Folder)
	}

	weekly, ok := cfg["weekly"]
	if !ok {
		t.Fatal("expected 'weekly' key in config")
	}
	if !weekly.Enabled {
		t.Error("expected weekly.Enabled=true")
	}
	if weekly.Format != "gggg-[W]ww" {
		t.Errorf("expected weekly.Format=gggg-[W]ww, got %q", weekly.Format)
	}

	monthly, ok := cfg["monthly"]
	if !ok {
		t.Fatal("expected 'monthly' key in config")
	}
	if monthly.Enabled {
		t.Error("expected monthly.Enabled=false")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	// Use a temp directory with no periodic-notes config
	svc := periodic.New(t.TempDir())
	cfg, err := svc.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig with missing file returned error: %v", err)
	}

	daily, ok := cfg["daily"]
	if !ok {
		t.Fatal("expected 'daily' default config")
	}
	if !daily.Enabled {
		t.Error("default daily should be enabled")
	}
	if daily.Format != "YYYY-MM-DD" {
		t.Errorf("expected default daily format YYYY-MM-DD, got %q", daily.Format)
	}
	if daily.Folder != "Daily Notes" {
		t.Errorf("expected default daily folder 'Daily Notes', got %q", daily.Folder)
	}

	weekly, ok := cfg["weekly"]
	if !ok {
		t.Fatal("expected 'weekly' default config")
	}
	if !weekly.Enabled {
		t.Error("default weekly should be enabled")
	}
}

func TestResolve_Daily(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("daily", 0)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Daily Notes/2024-01-15.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Daily_Offset(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("daily", -1)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Daily Notes/2024-01-14.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Weekly(t *testing.T) {
	// 2024-01-15 is Monday of ISO week 3, year 2024
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("weekly", 0)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Weekly Notes/2024-W03.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Monthly(t *testing.T) {
	// Monthly is disabled in fixture config but Resolve should still work
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("monthly", 0)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Monthly Notes/2024-01.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Quarterly(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("quarterly", 0)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Quarterly Notes/2024-Q1.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Yearly(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	got, err := svc.Resolve("yearly", 0)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Yearly Notes/2024.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_UnknownGranularity(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	_, err := svc.Resolve("hourly", 0)
	if err == nil {
		t.Error("expected error for unknown granularity, got nil")
	}
}

func TestRecentDates_Daily(t *testing.T) {
	svc := periodic.New(vaultRoot()).WithClock(fixedClock(2024, 1, 15))
	dates, err := svc.RecentDates("daily", 3)
	if err != nil {
		t.Fatalf("RecentDates returned error: %v", err)
	}
	if len(dates) != 3 {
		t.Fatalf("expected 3 dates, got %d", len(dates))
	}

	want := []time.Time{
		time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 13, 0, 0, 0, 0, time.UTC),
	}
	for i, d := range dates {
		if !d.Equal(want[i]) {
			t.Errorf("dates[%d]: got %v, want %v", i, d, want[i])
		}
	}
}
