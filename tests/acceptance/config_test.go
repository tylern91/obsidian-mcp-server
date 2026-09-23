package acceptance_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylern91/obsidian-mcp-server/internal/config"
)

func slicesEqualConfig(a, b []string) bool {
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

func TestConfig_Acceptance(t *testing.T) {
	t.Run("[AC-1] Load with only --vault set applies documented defaults for every other field", func(t *testing.T) {
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
			t.Errorf("LogLevel = %q, want warn", cfg.LogLevel)
		}
		if cfg.ReadOnly != false {
			t.Errorf("ReadOnly = %v, want false", cfg.ReadOnly)
		}
		if cfg.TrashRetentionDays != 30 {
			t.Errorf("TrashRetentionDays = %d, want 30", cfg.TrashRetentionDays)
		}
		if !slicesEqualConfig(cfg.Extensions, []string{".md", ".markdown", ".txt", ".canvas"}) {
			t.Errorf("Extensions = %v, want default set", cfg.Extensions)
		}
		if !slicesEqualConfig(cfg.IgnorePatterns, []string{".obsidian", ".git", "node_modules", ".DS_Store", ".trash"}) {
			t.Errorf("IgnorePatterns = %v, want default set", cfg.IgnorePatterns)
		}
		if cfg.Transport != "stdio" {
			t.Errorf("Transport = %q, want stdio", cfg.Transport)
		}
		if cfg.ClientCAPath != "" {
			t.Errorf("ClientCAPath = %q, want empty", cfg.ClientCAPath)
		}
	})

	t.Run("[AC-2] CLI flags override every default they correspond to", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{
			"--vault", vault,
			"--extensions", ".md,.txt",
			"--ignore", ".git",
			"--pretty",
			"--max-batch", "5",
			"--max-results", "50",
			"--log-level", "debug",
			"--read-only",
			"--trash-retention-days", "7",
			"--vault-name", "Custom Name",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.PrettyPrint || cfg.MaxBatch != 5 || cfg.MaxResults != 50 || cfg.LogLevel != "debug" {
			t.Errorf("cfg = %+v, want overridden pretty/max-batch/max-results/log-level", cfg)
		}
		if !slicesEqualConfig(cfg.Extensions, []string{".md", ".txt"}) {
			t.Errorf("Extensions = %v, want [.md .txt]", cfg.Extensions)
		}
		if !slicesEqualConfig(cfg.IgnorePatterns, []string{".git"}) {
			t.Errorf("IgnorePatterns = %v, want [.git]", cfg.IgnorePatterns)
		}
		if !cfg.ReadOnly || cfg.TrashRetentionDays != 7 {
			t.Errorf("ReadOnly/TrashRetentionDays = %v/%d, want true/7", cfg.ReadOnly, cfg.TrashRetentionDays)
		}
		if cfg.VaultName != "Custom Name" {
			t.Errorf("VaultName = %q, want %q", cfg.VaultName, "Custom Name")
		}
	})

	t.Run("[AC-3] every overridable field also honors its OBSIDIAN_ env var when no flag is given", func(t *testing.T) {
		vault := t.TempDir()
		t.Setenv("OBSIDIAN_VAULT_PATH", vault)
		t.Setenv("OBSIDIAN_READ_ONLY", "true")
		t.Setenv("OBSIDIAN_TRASH_RETENTION_DAYS", "3")
		t.Setenv("OBSIDIAN_MAX_BATCH", "7")
		t.Setenv("OBSIDIAN_MAX_RESULTS", "15")
		t.Setenv("OBSIDIAN_LOG_LEVEL", "info")
		t.Setenv("OBSIDIAN_PRETTY", "true")
		t.Setenv("OBSIDIAN_VAULT_NAME", "EnvVault")
		t.Setenv("OBSIDIAN_EXTENSIONS", ".md, .txt")
		t.Setenv("OBSIDIAN_IGNORE", ".git,.obsidian")

		cfg, err := config.Load([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.VaultPath != vault || !cfg.ReadOnly || cfg.TrashRetentionDays != 3 {
			t.Errorf("VaultPath/ReadOnly/TrashRetentionDays = %q/%v/%d", cfg.VaultPath, cfg.ReadOnly, cfg.TrashRetentionDays)
		}
		if cfg.MaxBatch != 7 || cfg.MaxResults != 15 || cfg.LogLevel != "info" || !cfg.PrettyPrint {
			t.Errorf("cfg = %+v, want env overrides applied", cfg)
		}
		if cfg.VaultName != "EnvVault" {
			t.Errorf("VaultName = %q, want EnvVault", cfg.VaultName)
		}
		if !slicesEqualConfig(cfg.Extensions, []string{".md", ".txt"}) {
			t.Errorf("Extensions = %v, want [.md .txt]", cfg.Extensions)
		}
		if !slicesEqualConfig(cfg.IgnorePatterns, []string{".git", ".obsidian"}) {
			t.Errorf("IgnorePatterns = %v, want [.git .obsidian]", cfg.IgnorePatterns)
		}
	})

	t.Run("[AC-4] an explicit CLI flag always wins over its OBSIDIAN_ env var counterpart", func(t *testing.T) {
		vault := t.TempDir()
		t.Setenv("OBSIDIAN_MAX_BATCH", "99")
		t.Setenv("OBSIDIAN_LOG_LEVEL", "error")
		t.Setenv("OBSIDIAN_TRANSPORT", "http")
		t.Setenv("OBSIDIAN_HTTP_PORT", "9999")

		cfg, err := config.Load([]string{
			"--vault", vault,
			"--max-batch", "3",
			"--log-level", "debug",
			"--transport", "stdio",
			"--http-port", "1234",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.MaxBatch != 3 {
			t.Errorf("MaxBatch = %d, want 3 (flag over env 99)", cfg.MaxBatch)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, want debug (flag over env 'error')", cfg.LogLevel)
		}
		if cfg.Transport != "stdio" {
			t.Errorf("Transport = %q, want stdio (flag over env 'http')", cfg.Transport)
		}
		if cfg.HTTPPort != 1234 {
			t.Errorf("HTTPPort = %d, want 1234 (flag over env 9999)", cfg.HTTPPort)
		}
	})

	t.Run("[AC-5] VaultName defaults to the vault directory's basename unless overridden", func(t *testing.T) {
		vault := filepath.Join(t.TempDir(), "MyVault")
		if err := os.Mkdir(vault, 0755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		cfg, err := config.Load([]string{"--vault", vault})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.VaultName != "MyVault" {
			t.Errorf("VaultName = %q, want MyVault", cfg.VaultName)
		}
	})

	t.Run("[AC-6] VaultPath is required, must exist, and must be a directory", func(t *testing.T) {
		if _, err := config.Load([]string{}); err == nil {
			t.Error("expected error for empty vault path, got nil")
		} else if want := "vault path is required: use --vault or OBSIDIAN_VAULT_PATH"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
		if _, err := config.Load([]string{"--vault", nonexistent}); err == nil {
			t.Error("expected error for non-existent vault path, got nil")
		} else if want := "vault path does not exist or is not a directory: " + nonexistent; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		dir := t.TempDir()
		filePath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := config.Load([]string{"--vault", filePath}); err == nil {
			t.Error("expected error when vault path is a file, got nil")
		} else if want := "vault path does not exist or is not a directory: " + filePath; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("[AC-7] Extensions and IgnorePatterns are comma-split and whitespace-trimmed from either a flag or an env var", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault, "--extensions", " .md , .txt , .canvas ", "--ignore", " .git , node_modules "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slicesEqualConfig(cfg.Extensions, []string{".md", ".txt", ".canvas"}) {
			t.Errorf("Extensions = %v, want trimmed split", cfg.Extensions)
		}
		if !slicesEqualConfig(cfg.IgnorePatterns, []string{".git", "node_modules"}) {
			t.Errorf("IgnorePatterns = %v, want trimmed split", cfg.IgnorePatterns)
		}
	})

	t.Run("[AC-8] MaxBatch, MaxResults, and TrashRetentionDays reject out-of-range values with a specific error message", func(t *testing.T) {
		vault := t.TempDir()
		if _, err := config.Load([]string{"--vault", vault, "--max-batch", "0"}); err == nil {
			t.Error("expected error for max-batch 0, got nil")
		} else if want := "max-batch must be at least 1, got 0"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		if _, err := config.Load([]string{"--vault", vault, "--max-results", "0"}); err == nil {
			t.Error("expected error for max-results 0, got nil")
		} else if want := "max-results must be at least 1, got 0"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		if _, err := config.Load([]string{"--vault", vault, "--trash-retention-days", "-1"}); err == nil {
			t.Error("expected error for negative trash-retention-days, got nil")
		}
	})

	t.Run("[AC-9] an unrecognized --log-level falls back to warn instead of erroring", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault, "--log-level", "verbose"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.LogLevel != "warn" {
			t.Errorf("LogLevel = %q, want warn for unrecognized level", cfg.LogLevel)
		}
	})

	t.Run("[AC-10] a malformed env var for a typed field (int or bool) errors and names the offending env var", func(t *testing.T) {
		vault := t.TempDir()

		t.Run("max batch", func(t *testing.T) {
			t.Setenv("OBSIDIAN_VAULT_PATH", vault)
			t.Setenv("OBSIDIAN_MAX_BATCH", "not-a-number")
			_, err := config.Load([]string{})
			if err == nil {
				t.Fatal("expected error for non-integer OBSIDIAN_MAX_BATCH, got nil")
			}
			if !strings.Contains(err.Error(), "OBSIDIAN_MAX_BATCH") {
				t.Errorf("error = %q, want mention of OBSIDIAN_MAX_BATCH", err.Error())
			}
		})

		t.Run("max results out of range", func(t *testing.T) {
			t.Setenv("OBSIDIAN_VAULT_PATH", vault)
			t.Setenv("OBSIDIAN_MAX_RESULTS", "-5")
			if _, err := config.Load([]string{}); err == nil {
				t.Fatal("expected error for negative OBSIDIAN_MAX_RESULTS, got nil")
			}
		})

		t.Run("pretty not a bool", func(t *testing.T) {
			t.Setenv("OBSIDIAN_VAULT_PATH", vault)
			t.Setenv("OBSIDIAN_PRETTY", "notabool")
			_, err := config.Load([]string{})
			if err == nil {
				t.Fatal("expected error for non-boolean OBSIDIAN_PRETTY, got nil")
			}
			if !strings.Contains(err.Error(), "OBSIDIAN_PRETTY") {
				t.Errorf("error = %q, want mention of OBSIDIAN_PRETTY", err.Error())
			}
		})
	})

	t.Run("[AC-11] --version short-circuits all other validation and returns the sentinel error", func(t *testing.T) {
		if _, err := config.Load([]string{"--version"}); !errors.Is(err, config.ErrVersionRequested) {
			t.Fatalf("Load(--version) error = %v, want ErrVersionRequested", err)
		}

		t.Setenv("OBSIDIAN_VAULT_PATH", "/nonexistent/path")
		if _, err := config.Load([]string{"--version"}); !errors.Is(err, config.ErrVersionRequested) {
			t.Fatalf("Load(--version) with bad vault env error = %v, want ErrVersionRequested", err)
		}

		if _, err := config.Load([]string{"--max-batch", "0", "--version"}); !errors.Is(err, config.ErrVersionRequested) {
			t.Fatalf("Load(--max-batch 0 --version) error = %v, want ErrVersionRequested", err)
		}
	})

	t.Run("[AC-12] Transport defaults to stdio, accepts http explicitly, and rejects any other value", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Transport != "stdio" {
			t.Errorf("Transport = %q, want stdio", cfg.Transport)
		}

		cfg, err = config.Load([]string{"--vault", vault, "--transport", "http"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Transport != "http" {
			t.Errorf("Transport = %q, want http", cfg.Transport)
		}

		for _, bad := range []string{"tcp", "grpc"} {
			_, err := config.Load([]string{"--vault", vault, "--transport", bad})
			if err == nil {
				t.Fatalf("expected error for transport %q, got nil", bad)
			}
			want := `transport must be "stdio" or "http", got "` + bad + `"`
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err.Error(), want)
			}
		}
	})

	t.Run("[AC-13] under http transport, HTTPBind/HTTPPort default sensibly and HTTPPort is validated to 1-65535", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault, "--transport", "http"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HTTPBind != "127.0.0.1" || cfg.HTTPPort != 8443 {
			t.Errorf("HTTPBind/HTTPPort = %q/%d, want 127.0.0.1/8443", cfg.HTTPBind, cfg.HTTPPort)
		}

		for _, port := range []string{"0", "-1", "65536"} {
			_, err := config.Load([]string{"--vault", vault, "--transport", "http", "--http-port", port})
			if err == nil {
				t.Fatalf("expected error for http-port %s, got nil", port)
			}
			if !strings.Contains(err.Error(), "http-port must be between 1 and 65535") {
				t.Errorf("error = %q, want it to mention the http-port range", err.Error())
			}
		}
	})

	t.Run("[AC-14] HTTPPort validation only applies when Transport is http", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault, "--http-port", "0"})
		if err != nil {
			t.Fatalf("unexpected error: %v (an out-of-range http-port must not error under stdio transport)", err)
		}
		if cfg.HTTPPort != 0 {
			t.Errorf("HTTPPort = %d, want 0 (unvalidated under stdio transport)", cfg.HTTPPort)
		}
	})

	t.Run("[AC-15] --allow-non-loopback requires both --allowed-hosts and --allowed-origins to be set, and succeeds once both are", func(t *testing.T) {
		vault := t.TempDir()

		_, err := config.Load([]string{
			"--vault", vault, "--transport", "http", "--allow-non-loopback",
			"--allowed-origins", "https://app.example.com",
		})
		if err == nil {
			t.Fatal("expected error when --allowed-hosts is missing, got nil")
		} else if want := "--allow-non-loopback requires --allowed-hosts to be set"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		_, err = config.Load([]string{
			"--vault", vault, "--transport", "http", "--allow-non-loopback",
			"--allowed-hosts", "internal.example.com",
		})
		if err == nil {
			t.Fatal("expected error when --allowed-origins is missing, got nil")
		} else if want := "--allow-non-loopback requires --allowed-origins to be set"; err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}

		cfg, err := config.Load([]string{
			"--vault", vault, "--transport", "http", "--allow-non-loopback",
			"--allowed-hosts", "internal.example.com",
			"--allowed-origins", "https://app.example.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.AllowNonLoopback {
			t.Error("AllowNonLoopback = false, want true")
		}
		if !slicesEqualConfig(cfg.AllowedHosts, []string{"internal.example.com"}) {
			t.Errorf("AllowedHosts = %v, want [internal.example.com]", cfg.AllowedHosts)
		}
		if !slicesEqualConfig(cfg.AllowedOrigins, []string{"https://app.example.com"}) {
			t.Errorf("AllowedOrigins = %v, want [https://app.example.com]", cfg.AllowedOrigins)
		}
	})

	t.Run("[AC-16] ClientCAPath defaults empty and is settable via flag", func(t *testing.T) {
		vault := t.TempDir()
		cfg, err := config.Load([]string{"--vault", vault})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ClientCAPath != "" {
			t.Errorf("ClientCAPath = %q, want empty", cfg.ClientCAPath)
		}

		cfg, err = config.Load([]string{"--vault", vault, "--client-ca", "/path/to/ca.pem"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ClientCAPath != "/path/to/ca.pem" {
			t.Errorf("ClientCAPath = %q, want /path/to/ca.pem", cfg.ClientCAPath)
		}
	})

	t.Run("[AC-17] the full HTTP/non-loopback env var group (transport, bind, port, allowlists, client CA) is honored together", func(t *testing.T) {
		vault := t.TempDir()
		t.Setenv("OBSIDIAN_VAULT_PATH", vault)
		t.Setenv("OBSIDIAN_TRANSPORT", "http")
		t.Setenv("OBSIDIAN_HTTP_BIND", "0.0.0.0")
		t.Setenv("OBSIDIAN_HTTP_PORT", "9443")
		t.Setenv("OBSIDIAN_ALLOW_NON_LOOPBACK", "true")
		t.Setenv("OBSIDIAN_ALLOWED_HOSTS", "internal.example.com,other.example.com")
		t.Setenv("OBSIDIAN_ALLOWED_ORIGINS", "https://app.example.com")
		t.Setenv("OBSIDIAN_CLIENT_CA", "/path/to/ca.pem")

		cfg, err := config.Load([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Transport != "http" || cfg.HTTPBind != "0.0.0.0" || cfg.HTTPPort != 9443 {
			t.Errorf("Transport/HTTPBind/HTTPPort = %q/%q/%d", cfg.Transport, cfg.HTTPBind, cfg.HTTPPort)
		}
		if !cfg.AllowNonLoopback {
			t.Error("AllowNonLoopback = false, want true")
		}
		if !slicesEqualConfig(cfg.AllowedHosts, []string{"internal.example.com", "other.example.com"}) {
			t.Errorf("AllowedHosts = %v, want two hosts", cfg.AllowedHosts)
		}
		if !slicesEqualConfig(cfg.AllowedOrigins, []string{"https://app.example.com"}) {
			t.Errorf("AllowedOrigins = %v, want [https://app.example.com]", cfg.AllowedOrigins)
		}
		if cfg.ClientCAPath != "/path/to/ca.pem" {
			t.Errorf("ClientCAPath = %q, want /path/to/ca.pem", cfg.ClientCAPath)
		}
	})
}
