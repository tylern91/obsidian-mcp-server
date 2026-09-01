package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrVersionRequested is returned by Load when the --version flag is passed.
// The caller should print the version and exit 0.
var ErrVersionRequested = errors.New("version requested")

// Config holds all configuration for the obsidian-mcp-server.
type Config struct {
	VaultPath          string
	Extensions         []string
	IgnorePatterns     []string
	PrettyPrint        bool
	MaxBatch           int
	MaxResults         int
	LogLevel           string
	InvalidLogLevel    string // non-empty when an unrecognized log level was given
	ReadOnly           bool
	TrashRetentionDays int
	VaultName          string

	Transport        string // "stdio" or "http"
	HTTPBind         string
	HTTPPort         int
	AllowNonLoopback bool
	AllowedHosts     []string
	AllowedOrigins   []string
	ClientCAPath     string // mTLS: path to a PEM file of trusted client CAs
}

// Load parses configuration from CLI flags, environment variables, and defaults.
// Priority: CLI flag > environment variable > default.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("obsidian-mcp", flag.ContinueOnError)

	// Define flags with defaults.
	showVersion := fs.Bool("version", false, "print version and exit")
	vaultPath := fs.String("vault", "", "path to Obsidian vault directory")
	extensions := fs.String("extensions", ".md,.markdown,.txt,.canvas", "comma-separated list of file extensions to index")
	ignorePatterns := fs.String("ignore", ".obsidian,.git,node_modules,.DS_Store,.trash", "comma-separated list of patterns to ignore")
	prettyPrint := fs.Bool("pretty", false, "enable pretty-printed JSON output")
	maxBatch := fs.Int("max-batch", 10, "maximum number of files per batch operation")
	maxResults := fs.Int("max-results", 20, "maximum number of search results")
	logLevel := fs.String("log-level", "warn", "log level: debug, info, warn, error")
	readOnly := fs.Bool("read-only", false, "disable all mutating tools (they are not registered, so clients never see them)")
	trashRetentionDays := fs.Int("trash-retention-days", 30, "days to keep trashed notes (.obsidian-mcp/trash) before they are pruned at startup")
	vaultName := fs.String("vault-name", "", "vault name used to build obsidian:// deep links in search/listing results (default: the vault directory's basename)")
	transport := fs.String("transport", "stdio", "MCP transport: stdio or http")
	httpBind := fs.String("http-bind", "127.0.0.1", "bind address for --transport http")
	httpPort := fs.Int("http-port", 8443, "port for --transport http")
	allowNonLoopback := fs.Bool("allow-non-loopback", false, "allow --http-bind to a non-loopback address (requires --allowed-hosts and --allowed-origins)")
	allowedHosts := fs.String("allowed-hosts", "", "comma-separated Host header allowlist for --transport http (required with --allow-non-loopback)")
	allowedOrigins := fs.String("allowed-origins", "", "comma-separated Origin header allowlist for --transport http (required with --allow-non-loopback)")
	clientCA := fs.String("client-ca", "", "path to a PEM file of trusted client CAs; enables mandatory mTLS for --transport http")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Short-circuit: --version takes priority over all other validation.
	if *showVersion {
		return nil, ErrVersionRequested
	}

	// Track which flags were explicitly set by the caller.
	explicitFlags := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	cfg := &Config{
		VaultPath:          *vaultPath,
		Extensions:         splitTrimmed(*extensions),
		IgnorePatterns:     splitTrimmed(*ignorePatterns),
		PrettyPrint:        *prettyPrint,
		MaxBatch:           *maxBatch,
		MaxResults:         *maxResults,
		LogLevel:           *logLevel,
		ReadOnly:           *readOnly,
		TrashRetentionDays: *trashRetentionDays,
		VaultName:          *vaultName,
		Transport:          *transport,
		HTTPBind:           *httpBind,
		HTTPPort:           *httpPort,
		AllowNonLoopback:   *allowNonLoopback,
		AllowedHosts:       splitTrimmed(*allowedHosts),
		AllowedOrigins:     splitTrimmed(*allowedOrigins),
		ClientCAPath:       *clientCA,
	}

	// Apply environment variable overrides for flags that were NOT explicitly set.
	envString(explicitFlags, "vault", "OBSIDIAN_VAULT_PATH", &cfg.VaultPath)
	envStringSlice(explicitFlags, "extensions", "OBSIDIAN_EXTENSIONS", &cfg.Extensions)
	envStringSlice(explicitFlags, "ignore", "OBSIDIAN_IGNORE", &cfg.IgnorePatterns)
	if err := envBool(explicitFlags, "pretty", "OBSIDIAN_PRETTY", &cfg.PrettyPrint); err != nil {
		return nil, err
	}
	if err := envInt(explicitFlags, "max-batch", "OBSIDIAN_MAX_BATCH", &cfg.MaxBatch); err != nil {
		return nil, err
	}
	if err := envInt(explicitFlags, "max-results", "OBSIDIAN_MAX_RESULTS", &cfg.MaxResults); err != nil {
		return nil, err
	}
	envString(explicitFlags, "log-level", "OBSIDIAN_LOG_LEVEL", &cfg.LogLevel)
	if err := envBool(explicitFlags, "read-only", "OBSIDIAN_READ_ONLY", &cfg.ReadOnly); err != nil {
		return nil, err
	}
	if err := envInt(explicitFlags, "trash-retention-days", "OBSIDIAN_TRASH_RETENTION_DAYS", &cfg.TrashRetentionDays); err != nil {
		return nil, err
	}
	envString(explicitFlags, "vault-name", "OBSIDIAN_VAULT_NAME", &cfg.VaultName)
	envString(explicitFlags, "transport", "OBSIDIAN_TRANSPORT", &cfg.Transport)
	envString(explicitFlags, "http-bind", "OBSIDIAN_HTTP_BIND", &cfg.HTTPBind)
	if err := envInt(explicitFlags, "http-port", "OBSIDIAN_HTTP_PORT", &cfg.HTTPPort); err != nil {
		return nil, err
	}
	if err := envBool(explicitFlags, "allow-non-loopback", "OBSIDIAN_ALLOW_NON_LOOPBACK", &cfg.AllowNonLoopback); err != nil {
		return nil, err
	}
	envStringSlice(explicitFlags, "allowed-hosts", "OBSIDIAN_ALLOWED_HOSTS", &cfg.AllowedHosts)
	envStringSlice(explicitFlags, "allowed-origins", "OBSIDIAN_ALLOWED_ORIGINS", &cfg.AllowedOrigins)
	envString(explicitFlags, "client-ca", "OBSIDIAN_CLIENT_CA", &cfg.ClientCAPath)

	// Normalize LogLevel: default to "warn" if unrecognized, and record the bad value.
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		cfg.InvalidLogLevel = cfg.LogLevel
		cfg.LogLevel = "warn"
	}

	// Trim VaultPath before empty check to reject whitespace-only values.
	cfg.VaultPath = strings.TrimSpace(cfg.VaultPath)

	// Validate required fields.
	if cfg.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required: use --vault or OBSIDIAN_VAULT_PATH")
	}

	info, err := os.Stat(cfg.VaultPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("vault path does not exist or is not a directory: %s", cfg.VaultPath)
	}

	cfg.VaultName = strings.TrimSpace(cfg.VaultName)
	if cfg.VaultName == "" {
		cfg.VaultName = filepath.Base(cfg.VaultPath)
	}

	if cfg.MaxBatch < 1 {
		return nil, fmt.Errorf("max-batch must be at least 1, got %d", cfg.MaxBatch)
	}

	if cfg.MaxResults < 1 {
		return nil, fmt.Errorf("max-results must be at least 1, got %d", cfg.MaxResults)
	}

	if cfg.TrashRetentionDays < 0 {
		return nil, fmt.Errorf("trash-retention-days must be at least 0, got %d", cfg.TrashRetentionDays)
	}

	switch cfg.Transport {
	case "stdio", "http":
		// valid
	default:
		return nil, fmt.Errorf("transport must be \"stdio\" or \"http\", got %q", cfg.Transport)
	}

	if cfg.Transport == "http" {
		if cfg.HTTPPort < 1 || cfg.HTTPPort > 65535 {
			return nil, fmt.Errorf("http-port must be between 1 and 65535, got %d", cfg.HTTPPort)
		}
		if cfg.AllowNonLoopback {
			if len(cfg.AllowedHosts) == 0 {
				return nil, fmt.Errorf("--allow-non-loopback requires --allowed-hosts to be set")
			}
			if len(cfg.AllowedOrigins) == 0 {
				return nil, fmt.Errorf("--allow-non-loopback requires --allowed-origins to be set")
			}
		}
	}

	return cfg, nil
}

// SlogLevel returns the slog.Level corresponding to the configured log level.
func (c *Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default: // "warn" and any normalized fallback
		return slog.LevelWarn
	}
}

// splitTrimmed splits s on commas and trims whitespace from each element,
// discarding any empty strings that result.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// envString sets *dst to the value of the environment variable env when the
// flag was not explicitly provided by the caller.
func envString(explicit map[string]bool, flag, env string, dst *string) {
	if !explicit[flag] {
		if v := os.Getenv(env); v != "" {
			*dst = v
		}
	}
}

// envStringSlice sets *dst to the splitTrimmed value of env when the flag was
// not explicitly provided by the caller.
func envStringSlice(explicit map[string]bool, flag, env string, dst *[]string) {
	if !explicit[flag] {
		if v := os.Getenv(env); v != "" {
			*dst = splitTrimmed(v)
		}
	}
}

// envBool sets *dst to the parsed boolean value of env when the flag was not
// explicitly provided. Returns an error if the env value cannot be parsed.
func envBool(explicit map[string]bool, flag, env string, dst *bool) error {
	if !explicit[flag] {
		if v := os.Getenv(env); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("invalid %s value %q: %w", env, v, err)
			}
			*dst = b
		}
	}
	return nil
}

// envInt sets *dst to the parsed integer value of env when the flag was not
// explicitly provided. Returns an error if the env value cannot be parsed.
func envInt(explicit map[string]bool, flag, env string, dst *int) error {
	if !explicit[flag] {
		if v := os.Getenv(env); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid %s value %q: %w", env, v, err)
			}
			*dst = n
		}
	}
	return nil
}
