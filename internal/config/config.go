package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the obsidian-mcp-server.
type Config struct {
	VaultPath      string
	Extensions     []string
	IgnorePatterns []string
	PrettyPrint    bool
	MaxBatch       int
	MaxResults     int
	LogLevel       string
}

// Load parses configuration from CLI flags, environment variables, and defaults.
// Priority: CLI flag > environment variable > default.
func Load(args []string) (*Config, error) {
	fs := flag.NewFlagSet("obsidian-mcp", flag.ContinueOnError)

	// Define flags with defaults.
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
	if !explicitFlags["vault"] {
		if v := os.Getenv("OBSIDIAN_VAULT_PATH"); v != "" {
			cfg.VaultPath = v
		}
	}

	if !explicitFlags["extensions"] {
		if v := os.Getenv("OBSIDIAN_EXTENSIONS"); v != "" {
			cfg.Extensions = splitTrimmed(v)
		}
	}

	if !explicitFlags["ignore"] {
		if v := os.Getenv("OBSIDIAN_IGNORE"); v != "" {
			cfg.IgnorePatterns = splitTrimmed(v)
		}
	}

	if !explicitFlags["pretty"] {
		if v := os.Getenv("OBSIDIAN_PRETTY"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.PrettyPrint = b
			}
		}
	}

	if !explicitFlags["max-batch"] {
		if v := os.Getenv("OBSIDIAN_MAX_BATCH"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.MaxBatch = n
			}
			// silently ignore invalid integers — default remains
		}
	}

	if !explicitFlags["max-results"] {
		if v := os.Getenv("OBSIDIAN_MAX_RESULTS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				cfg.MaxResults = n
			}
			// silently ignore invalid integers — default remains
		}
	}

	if !explicitFlags["log-level"] {
		if v := os.Getenv("OBSIDIAN_LOG_LEVEL"); v != "" {
			cfg.LogLevel = v
		}
	}

	// Normalize LogLevel: default to "warn" if unrecognized.
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
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
