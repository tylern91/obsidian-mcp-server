package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultPath != vault {
		t.Errorf("VaultPath = %q, want %q", cfg.VaultPath, vault)
	}
	if cfg.PrettyPrint != false {
		t.Errorf("PrettyPrint = %v, want false", cfg.PrettyPrint)
	}
	if cfg.MaxBatch != 10 {
		t.Errorf("MaxBatch = %d, want 10", cfg.MaxBatch)
	}
	if cfg.MaxResults != 20 {
		t.Errorf("MaxResults = %d, want 20", cfg.MaxResults)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
	wantExt := []string{".md", ".markdown", ".txt", ".canvas"}
	if !slicesEqual(cfg.Extensions, wantExt) {
		t.Errorf("Extensions = %v, want %v", cfg.Extensions, wantExt)
	}
	wantIgnore := []string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"}
	if !slicesEqual(cfg.IgnorePatterns, wantIgnore) {
		t.Errorf("IgnorePatterns = %v, want %v", cfg.IgnorePatterns, wantIgnore)
	}
}

func TestLoad_CLIFlagOverridesDefaults(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{
		"--vault", vault,
		"--extensions", ".md,.txt",
		"--ignore", ".git",
		"--pretty",
		"--max-batch", "5",
		"--max-results", "50",
		"--log-level", "debug",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PrettyPrint != true {
		t.Errorf("PrettyPrint = %v, want true", cfg.PrettyPrint)
	}
	if cfg.MaxBatch != 5 {
		t.Errorf("MaxBatch = %d, want 5", cfg.MaxBatch)
	}
	if cfg.MaxResults != 50 {
		t.Errorf("MaxResults = %d, want 50", cfg.MaxResults)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if !slicesEqual(cfg.Extensions, []string{".md", ".txt"}) {
		t.Errorf("Extensions = %v, want [.md .txt]", cfg.Extensions)
	}
	if !slicesEqual(cfg.IgnorePatterns, []string{".git"}) {
		t.Errorf("IgnorePatterns = %v, want [.git]", cfg.IgnorePatterns)
	}
}

func TestLoad_EnvVarOverridesDefaults(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_MAX_BATCH", "7")
	t.Setenv("OBSIDIAN_MAX_RESULTS", "15")
	t.Setenv("OBSIDIAN_LOG_LEVEL", "info")
	t.Setenv("OBSIDIAN_PRETTY", "true")

	cfg, err := config.Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultPath != vault {
		t.Errorf("VaultPath = %q, want %q", cfg.VaultPath, vault)
	}
	if cfg.MaxBatch != 7 {
		t.Errorf("MaxBatch = %d, want 7", cfg.MaxBatch)
	}
	if cfg.MaxResults != 15 {
		t.Errorf("MaxResults = %d, want 15", cfg.MaxResults)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.PrettyPrint != true {
		t.Errorf("PrettyPrint = %v, want true", cfg.PrettyPrint)
	}
}

func TestLoad_CLIFlagTakesPrecedenceOverEnvVar(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_MAX_BATCH", "99")
	t.Setenv("OBSIDIAN_LOG_LEVEL", "error")

	cfg, err := config.Load([]string{
		"--vault", vault,
		"--max-batch", "3",
		"--log-level", "debug",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxBatch != 3 {
		t.Errorf("MaxBatch = %d, want 3 (CLI flag should win over env var 99)", cfg.MaxBatch)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug (CLI flag should win over env var 'error')", cfg.LogLevel)
	}
}

func TestLoad_ErrorEmptyVaultPath(t *testing.T) {
	_, err := config.Load([]string{})
	if err == nil {
		t.Fatal("expected error for empty vault path, got nil")
	}
	want := "vault path is required: use --vault or OBSIDIAN_VAULT_PATH"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestLoad_ErrorVaultPathDoesNotExist(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := config.Load([]string{"--vault", nonexistent})
	if err == nil {
		t.Fatal("expected error for non-existent vault path, got nil")
	}
	want := "vault path does not exist or is not a directory: " + nonexistent
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestLoad_ErrorVaultPathIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := config.Load([]string{"--vault", filePath})
	if err == nil {
		t.Fatal("expected error when vault path is a file, got nil")
	}
	want := "vault path does not exist or is not a directory: " + filePath
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestLoad_ExtensionsFlagCommaSeparated(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--extensions", " .md , .txt , .canvas "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{".md", ".txt", ".canvas"}
	if !slicesEqual(cfg.Extensions, want) {
		t.Errorf("Extensions = %v, want %v", cfg.Extensions, want)
	}
}

func TestLoad_ExtensionsEnvVarCommaSeparated(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_EXTENSIONS", ".md, .txt")

	cfg, err := config.Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{".md", ".txt"}
	if !slicesEqual(cfg.Extensions, want) {
		t.Errorf("Extensions = %v, want %v", cfg.Extensions, want)
	}
}

func TestLoad_IgnorePatternsFlagCommaSeparated(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--ignore", " .git , node_modules "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{".git", "node_modules"}
	if !slicesEqual(cfg.IgnorePatterns, want) {
		t.Errorf("IgnorePatterns = %v, want %v", cfg.IgnorePatterns, want)
	}
}

func TestLoad_IgnorePatternsEnvVarCommaSeparated(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_IGNORE", ".git,.obsidian")

	cfg, err := config.Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{".git", ".obsidian"}
	if !slicesEqual(cfg.IgnorePatterns, want) {
		t.Errorf("IgnorePatterns = %v, want %v", cfg.IgnorePatterns, want)
	}
}

func TestLoad_ErrorMaxBatchLessThanOne(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--max-batch", "0"})
	if err == nil {
		t.Fatal("expected error for max-batch < 1, got nil")
	}
}

func TestLoad_ErrorMaxResultsLessThanOne(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--max-results", "0"})
	if err == nil {
		t.Fatal("expected error for max-results < 1, got nil")
	}
}

func TestLoad_UnrecognizedLogLevelDefaultsToWarn(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--log-level", "verbose"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn for unrecognized level", cfg.LogLevel)
	}
}

// slicesEqual reports whether two string slices are equal in length and content.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
