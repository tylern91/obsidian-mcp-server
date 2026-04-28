package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// ErrVersionRequested is returned by Load when the --version flag is passed.
// The caller should print the version and exit 0.
var ErrVersionRequested = errors.New("version requested")

// Config holds all configuration for the obsidian-mcp-server.
type Config struct {
	VaultPath       string
	Extensions      []string
	IgnorePatterns  []string
	PrettyPrint     bool
	MaxBatch        int
	MaxResults      int
	LogLevel        string
	InvalidLogLevel string // non-empty when an unrecognized log level was given
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
		VaultPath:      *vaultPath,
		Extensions:     splitTrimmed(*extensions),
		IgnorePatterns: splitTrimmed(*ignorePatterns),
		PrettyPrint:    *prettyPrint,
		MaxBatch:       *maxBatch,
		MaxResults:     *maxResults,
		LogLevel:       *logLevel,
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

	if cfg.MaxBatch < 1 {
		return nil, fmt.Errorf("max-batch must be at least 1, got %d", cfg.MaxBatch)
	}

	if cfg.MaxResults < 1 {
		return nil, fmt.Errorf("max-results must be at least 1, got %d", cfg.MaxResults)
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
