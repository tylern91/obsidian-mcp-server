package periodic

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// GranularityConfig holds config for one periodic note granularity.
type GranularityConfig struct {
	Enabled  bool   `json:"enabled"`
	Format   string `json:"format"` // moment.js format string
	Folder   string `json:"folder"`
	Template string `json:"template"`
}

// Config holds configs for all five granularities.
type Config map[string]GranularityConfig // key: "daily","weekly","monthly","quarterly","yearly"

// defaultConfig is returned when .obsidian/plugins/periodic-notes/data.json is missing.
var defaultConfig = Config{
	"daily":     {Enabled: true, Format: "YYYY-MM-DD", Folder: "Daily Notes"},
	"weekly":    {Enabled: true, Format: "gggg-[W]ww", Folder: "Weekly Notes"},
	"monthly":   {Enabled: false, Format: "YYYY-MM", Folder: "Monthly Notes"},
	"quarterly": {Enabled: false, Format: "YYYY-[Q]Q", Folder: "Quarterly Notes"},
	"yearly":    {Enabled: false, Format: "YYYY", Folder: "Yearly Notes"},
}

// Service resolves periodic note paths.
type Service struct {
	vaultRoot   string
	clock       func() time.Time
	configCache *Config // nil = not yet loaded
}

// New creates a Service using time.Now as the clock.
func New(vaultRoot string) *Service {
	return &Service{
		vaultRoot: vaultRoot,
		clock:     time.Now,
	}
}

// WithClock returns a new Service with an injectable clock (for tests).
func (s *Service) WithClock(fn func() time.Time) *Service {
	return &Service{
		vaultRoot: s.vaultRoot,
		clock:     fn,
	}
}

// LoadConfig reads .obsidian/plugins/periodic-notes/data.json.
// If the file is missing, returns built-in defaults (daily enabled, YYYY-MM-DD, "Daily Notes").
// Results are cached on the Service after the first successful load.
func (s *Service) LoadConfig() (Config, error) {
	if s.configCache != nil {
		// Return a shallow copy so callers cannot mutate the cache.
		out := make(Config, len(*s.configCache))
		mergeStringMap(out, *s.configCache)
		return out, nil
	}

	configPath := filepath.Join(s.vaultRoot, ".obsidian", "plugins", "periodic-notes", "data.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return a copy of the defaults and cache them.
			out := make(Config, len(defaultConfig))
			mergeStringMap(out, defaultConfig)
			s.configCache = &out
			return out, nil
		}
		return nil, fmt.Errorf("periodic: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("periodic: parse config: %w", err)
	}
	s.configCache = &cfg
	// Return a copy so callers cannot mutate the cache.
	out := make(Config, len(cfg))
	mergeStringMap(out, cfg)
	return out, nil
}

// mergeStringMap copies all entries from src into dst.
func mergeStringMap(dst, src Config) {
	for k, v := range src {
		dst[k] = v
	}
}

// formatMoment converts a moment.js format string and a time.Time into the
// formatted filename stem. It walks the format token by token as a state machine.
func formatMoment(format string, t time.Time) (string, error) {
	isoYear, isoWeek := t.ISOWeek()
	quarter := (int(t.Month())-1)/3 + 1

	var sb strings.Builder
	i := 0
	runes := []rune(format)
	n := len(runes)

	for i < n {
		r := runes[i]

		// Literal bracket escape: [anything]
		if r == '[' {
			i++
			for i < n && runes[i] != ']' {
				sb.WriteRune(runes[i])
				i++
			}
			if i < n {
				i++ // consume ']'
			}
			continue
		}

		// Try to match known tokens longest-first
		remaining := string(runes[i:])

		switch {
		case strings.HasPrefix(remaining, "YYYY"):
			fmt.Fprintf(&sb, "%04d", t.Year())
			i += 4
		case strings.HasPrefix(remaining, "YY"):
			fmt.Fprintf(&sb, "%02d", t.Year()%100)
			i += 2
		case strings.HasPrefix(remaining, "MM"):
			fmt.Fprintf(&sb, "%02d", int(t.Month()))
			i += 2
		case strings.HasPrefix(remaining, "M"):
			fmt.Fprintf(&sb, "%d", int(t.Month()))
			i++
		case strings.HasPrefix(remaining, "DD"):
			fmt.Fprintf(&sb, "%02d", t.Day())
			i += 2
		case strings.HasPrefix(remaining, "D"):
			fmt.Fprintf(&sb, "%d", t.Day())
			i++
		case strings.HasPrefix(remaining, "gggg"):
			fmt.Fprintf(&sb, "%04d", isoYear)
			i += 4
		case strings.HasPrefix(remaining, "ggg"):
			fmt.Fprintf(&sb, "%03d", isoYear%1000)
			i += 3
		case strings.HasPrefix(remaining, "ww"):
			fmt.Fprintf(&sb, "%02d", isoWeek)
			i += 2
		case strings.HasPrefix(remaining, "w"):
			fmt.Fprintf(&sb, "%d", isoWeek)
			i++
		case strings.HasPrefix(remaining, "Q"):
			fmt.Fprintf(&sb, "%d", quarter)
			i++
		default:
			// Pass through literal characters (e.g. '-')
			sb.WriteRune(r)
			i++
		}
	}

	return sb.String(), nil
}

// applyOffset applies an offset in the granularity's natural unit to t.
func applyOffset(granularity string, t time.Time, offset int) (time.Time, error) {
	switch granularity {
	case "daily":
		return t.AddDate(0, 0, offset), nil
	case "weekly":
		return t.AddDate(0, 0, offset*7), nil
	case "monthly":
		return t.AddDate(0, offset, 0), nil
	case "quarterly":
		return t.AddDate(0, offset*3, 0), nil
	case "yearly":
		return t.AddDate(offset, 0, 0), nil
	default:
		return t, fmt.Errorf("periodic: unknown granularity %q", granularity)
	}
}

// Resolve computes the vault-relative path for a periodic note.
// granularity: "daily","weekly","monthly","quarterly","yearly"
// offset: 0=current, -1=previous, +1=next (in the granularity's unit)
// Returns: "Daily Notes/2024-01-15.md" style path
func (s *Service) Resolve(granularity string, offset int) (string, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return "", err
	}

	gc, ok := cfg[granularity]
	if !ok {
		return "", fmt.Errorf("periodic: unknown granularity %q", granularity)
	}
	if gc.Folder == "" {
		return "", fmt.Errorf("periodic: folder not configured for granularity %q", granularity)
	}

	t := s.clock()
	t, err = applyOffset(granularity, t, offset)
	if err != nil {
		return "", err
	}

	stem, err := formatMoment(gc.Format, t)
	if err != nil {
		return "", err
	}

	return path.Join(gc.Folder, stem+".md"), nil
}

// RecentDates returns the `count` most recent dates (descending) ending at now.
func (s *Service) RecentDates(granularity string, count int) ([]time.Time, error) {
	if _, err := applyOffset(granularity, time.Time{}, 0); err != nil {
		return nil, fmt.Errorf("periodic: unknown granularity %q", granularity)
	}

	now := s.clock()
	dates := make([]time.Time, count)
	for i := 0; i < count; i++ {
		t, err := applyOffset(granularity, now, -i)
		if err != nil {
			return nil, err
		}
		dates[i] = t
	}
	return dates, nil
}
