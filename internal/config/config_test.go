package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if cfg.ReadOnly != false {
		t.Errorf("ReadOnly = %v, want false", cfg.ReadOnly)
	}
	if cfg.TrashRetentionDays != 30 {
		t.Errorf("TrashRetentionDays = %d, want 30", cfg.TrashRetentionDays)
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

func TestLoad_ReadOnlyAndTrashRetentionFlags(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{
		"--vault", vault,
		"--read-only",
		"--trash-retention-days", "7",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ReadOnly != true {
		t.Errorf("ReadOnly = %v, want true", cfg.ReadOnly)
	}
	if cfg.TrashRetentionDays != 7 {
		t.Errorf("TrashRetentionDays = %d, want 7", cfg.TrashRetentionDays)
	}
}

func TestLoad_ReadOnlyAndTrashRetentionEnvVars(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_READ_ONLY", "true")
	t.Setenv("OBSIDIAN_TRASH_RETENTION_DAYS", "3")

	cfg, err := config.Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ReadOnly != true {
		t.Errorf("ReadOnly = %v, want true", cfg.ReadOnly)
	}
	if cfg.TrashRetentionDays != 3 {
		t.Errorf("TrashRetentionDays = %d, want 3", cfg.TrashRetentionDays)
	}
}

func TestLoad_VaultNameDefaultsToDirBasename(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "MyVault")
	if err := os.Mkdir(vault, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg, err := config.Load([]string{"--vault", vault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultName != "MyVault" {
		t.Errorf("VaultName = %q, want %q", cfg.VaultName, "MyVault")
	}
}

func TestLoad_VaultNameFlagOverridesBasename(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--vault-name", "Custom Name"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultName != "Custom Name" {
		t.Errorf("VaultName = %q, want %q", cfg.VaultName, "Custom Name")
	}
}

func TestLoad_VaultNameEnvVar(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_VAULT_NAME", "EnvVault")
	cfg, err := config.Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VaultName != "EnvVault" {
		t.Errorf("VaultName = %q, want %q", cfg.VaultName, "EnvVault")
	}
}

func TestLoad_ErrorTrashRetentionDaysNegative(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--trash-retention-days", "-1"})
	if err == nil {
		t.Fatal("expected error for negative trash-retention-days, got nil")
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
	want := "max-batch must be at least 1, got 0"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestLoad_ErrorMaxResultsLessThanOne(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--max-results", "0"})
	if err == nil {
		t.Fatal("expected error for max-results < 1, got nil")
	}
	want := "max-results must be at least 1, got 0"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
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

func TestLoad_ErrorInvalidEnvVarMaxBatch(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_MAX_BATCH", "not-a-number")

	_, err := config.Load([]string{})
	if err == nil {
		t.Fatal("expected error for non-integer OBSIDIAN_MAX_BATCH, got nil")
	}
	if !strings.Contains(err.Error(), "OBSIDIAN_MAX_BATCH") {
		t.Errorf("error = %q, want mention of OBSIDIAN_MAX_BATCH", err.Error())
	}
}

func TestLoad_ErrorInvalidEnvVarMaxResults(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_MAX_RESULTS", "-5")

	_, err := config.Load([]string{})
	if err == nil {
		t.Fatal("expected error for negative OBSIDIAN_MAX_RESULTS, got nil")
	}
}

func TestLoad_ErrorInvalidEnvVarPretty(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)
	t.Setenv("OBSIDIAN_PRETTY", "notabool")

	_, err := config.Load([]string{})
	if err == nil {
		t.Fatal("expected error for non-boolean OBSIDIAN_PRETTY, got nil")
	}
	if !strings.Contains(err.Error(), "OBSIDIAN_PRETTY") {
		t.Errorf("error = %q, want mention of OBSIDIAN_PRETTY", err.Error())
	}
}

func TestLoad_VersionFlagReturnsSentinel(t *testing.T) {
	_, err := config.Load([]string{"--version"})
	if !errors.Is(err, config.ErrVersionRequested) {
		t.Fatalf("Load(--version) error = %v, want ErrVersionRequested", err)
	}
}

func TestLoad_VersionFlagShortCircuitsValidation(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT_PATH", "/nonexistent/path")
	_, err := config.Load([]string{"--version"})
	if !errors.Is(err, config.ErrVersionRequested) {
		t.Fatalf("Load(--version) with bad vault env error = %v, want ErrVersionRequested", err)
	}
}

func TestLoad_VersionFlagIgnoresOtherInvalidFlags(t *testing.T) {
	_, err := config.Load([]string{"--max-batch", "0", "--version"})
	if !errors.Is(err, config.ErrVersionRequested) {
		t.Fatalf("Load(--max-batch 0 --version) error = %v, want ErrVersionRequested", err)
	}
}

func TestLoad_TransportDefaultsToStdio(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "stdio")
	}
}

func TestLoad_TransportExplicitHTTP(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--transport", "http"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport != "http" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "http")
	}
}

func TestLoad_TransportRejectsInvalidValue(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--transport", "tcp"})
	if err == nil {
		t.Fatal("expected error for invalid transport, got nil")
	}
	wantMsg := `transport must be "stdio" or "http", got "tcp"`
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
}

func TestLoad_TransportRejectsInvalidValue_grpc(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{"--vault", vault, "--transport", "grpc"})
	if err == nil {
		t.Fatal("expected error for invalid transport, got nil")
	}
	wantMsg := `transport must be "stdio" or "http", got "grpc"`
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
}

func TestLoad_HTTPBindAndPortDefaults(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--transport", "http"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPBind != "127.0.0.1" {
		t.Errorf("HTTPBind = %q, want %q", cfg.HTTPBind, "127.0.0.1")
	}
	if cfg.HTTPPort != 8443 {
		t.Errorf("HTTPPort = %d, want 8443", cfg.HTTPPort)
	}
}

func TestLoad_HTTPPortRangeValidation(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"too large", "65536"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vault := t.TempDir()
			_, err := config.Load([]string{"--vault", vault, "--transport", "http", "--http-port", tt.port})
			if err == nil {
				t.Fatalf("expected error for http-port %s, got nil", tt.port)
			}
			if !strings.Contains(err.Error(), "http-port must be between 1 and 65535") {
				t.Errorf("error = %q, want it to mention the http-port range", err.Error())
			}
		})
	}
}

func TestLoad_HTTPPortValidationOnlyAppliesToHTTPTransport(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--http-port", "0"})
	if err != nil {
		t.Fatalf("unexpected error: %v (an out-of-range http-port must not error under --transport stdio)", err)
	}
	if cfg.HTTPPort != 0 {
		t.Errorf("HTTPPort = %d, want 0 (unvalidated under stdio transport)", cfg.HTTPPort)
	}
}

func TestLoad_AllowNonLoopbackRequiresAllowedHosts(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{
		"--vault", vault,
		"--transport", "http",
		"--allow-non-loopback",
		"--allowed-origins", "https://app.example.com",
	})
	if err == nil {
		t.Fatal("expected error when --allowed-hosts is missing, got nil")
	}
	wantMsg := "--allow-non-loopback requires --allowed-hosts to be set"
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
}

func TestLoad_AllowNonLoopbackRequiresAllowedOrigins(t *testing.T) {
	vault := t.TempDir()
	_, err := config.Load([]string{
		"--vault", vault,
		"--transport", "http",
		"--allow-non-loopback",
		"--allowed-hosts", "internal.example.com",
	})
	if err == nil {
		t.Fatal("expected error when --allowed-origins is missing, got nil")
	}
	wantMsg := "--allow-non-loopback requires --allowed-origins to be set"
	if err.Error() != wantMsg {
		t.Errorf("error = %q, want %q", err.Error(), wantMsg)
	}
}

func TestLoad_AllowNonLoopbackSucceedsWithBothAllowlists(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{
		"--vault", vault,
		"--transport", "http",
		"--allow-non-loopback",
		"--allowed-hosts", "internal.example.com",
		"--allowed-origins", "https://app.example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AllowNonLoopback {
		t.Error("AllowNonLoopback = false, want true")
	}
	wantHosts := []string{"internal.example.com"}
	if !slicesEqual(cfg.AllowedHosts, wantHosts) {
		t.Errorf("AllowedHosts = %v, want %v", cfg.AllowedHosts, wantHosts)
	}
	wantOrigins := []string{"https://app.example.com"}
	if !slicesEqual(cfg.AllowedOrigins, wantOrigins) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, wantOrigins)
	}
}

func TestLoad_ClientCAPathDefaultsEmpty(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientCAPath != "" {
		t.Errorf("ClientCAPath = %q, want empty", cfg.ClientCAPath)
	}
}

func TestLoad_ClientCAPathFromFlag(t *testing.T) {
	vault := t.TempDir()
	cfg, err := config.Load([]string{"--vault", vault, "--client-ca", "/path/to/ca.pem"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientCAPath != "/path/to/ca.pem" {
		t.Errorf("ClientCAPath = %q, want %q", cfg.ClientCAPath, "/path/to/ca.pem")
	}
}

func TestLoad_Phase6EnvVarOverrides(t *testing.T) {
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
	if cfg.Transport != "http" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "http")
	}
	if cfg.HTTPBind != "0.0.0.0" {
		t.Errorf("HTTPBind = %q, want %q", cfg.HTTPBind, "0.0.0.0")
	}
	if cfg.HTTPPort != 9443 {
		t.Errorf("HTTPPort = %d, want 9443", cfg.HTTPPort)
	}
	if !cfg.AllowNonLoopback {
		t.Error("AllowNonLoopback = false, want true")
	}
	wantHosts := []string{"internal.example.com", "other.example.com"}
	if !slicesEqual(cfg.AllowedHosts, wantHosts) {
		t.Errorf("AllowedHosts = %v, want %v", cfg.AllowedHosts, wantHosts)
	}
	wantOrigins := []string{"https://app.example.com"}
	if !slicesEqual(cfg.AllowedOrigins, wantOrigins) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, wantOrigins)
	}
	if cfg.ClientCAPath != "/path/to/ca.pem" {
		t.Errorf("ClientCAPath = %q, want %q", cfg.ClientCAPath, "/path/to/ca.pem")
	}
}

func TestLoad_TransportCLIFlagTakesPrecedenceOverEnvVar(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_TRANSPORT", "http")
	t.Setenv("OBSIDIAN_HTTP_PORT", "9999")

	cfg, err := config.Load([]string{
		"--vault", vault,
		"--transport", "stdio",
		"--http-port", "1234",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("Transport = %q, want stdio (CLI flag should win over env var 'http')", cfg.Transport)
	}
	if cfg.HTTPPort != 1234 {
		t.Errorf("HTTPPort = %d, want 1234 (CLI flag should win over env var 9999)", cfg.HTTPPort)
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
