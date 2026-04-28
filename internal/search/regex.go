package search

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultRegexLimit        = 20
	defaultMaxMatchesPerFile = 5
)

// RegexOptions controls behaviour of SearchRegex.
type RegexOptions struct {
	// Pattern is either a RE2 regex or a filepath.Match-style glob (when IsGlob=true).
	Pattern string

	// IsGlob treats Pattern as a filepath.Match glob and converts it to an
	// anchored regex before searching.
	IsGlob bool

	// Scope selects what to match against: "path", "content", or "both".
	// Default (empty string) is treated as "content".
	Scope string

	// Limit caps the total number of results returned. 0 uses the default (20).
	Limit int

	// MaxMatchesPerFile caps the number of matching lines collected per file.
	// 0 uses the default (5).
	MaxMatchesPerFile int
}

// RegexMatch represents a single matching line within a file.
type RegexMatch struct {
	Line    int    `json:"line"`    // 1-indexed line number
	Snippet string `json:"snippet"` // trimmed content of the matching line
}

// RegexResult holds all matches for a single vault file.
type RegexResult struct {
	Path    string       `json:"path"`    // relative vault path (forward slashes)
	Matches []RegexMatch `json:"matches"` // per-file match lines; empty for path-only matches
}

// globToRegex converts a filepath.Match-style glob pattern into an anchored RE2
// regex string. The rules are:
//
//	**/  → (.*/)?    (matches zero or more path segments, including none)
//	**   → .*        (matches everything, including path separators)
//	*    → [^/]*     (matches within a single directory level)
//	?    → [^/]      (matches one character that is not a separator)
//	all other regex metacharacters are escaped
func globToRegex(glob string) string {
	var sb strings.Builder
	sb.WriteString("^")

	// We need to handle "**/" before "**" before "*", so we walk byte by byte.
	i := 0
	for i < len(glob) {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				// Check for "**/" — zero or more directory segments.
				if i+2 < len(glob) && glob[i+2] == '/' {
					sb.WriteString("(.*/)?")
					i += 3
					continue
				}
				// "**" without following "/" — matches everything.
				sb.WriteString(".*")
				i += 2
				continue
			}
			sb.WriteString("[^/]*")
		case '?':
			sb.WriteString("[^/]")
		default:
			// Escape any RE2 metacharacter.
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
		i++
	}

	sb.WriteString("$")
	return sb.String()
}

// SearchRegex searches vault notes using a regular expression (or glob) pattern.
//
// Scope values:
//   - "path"    – match the relative file path only; Matches is empty for hits
//   - "content" – scan file content line by line; Matches contains matching lines
//   - "both"    – match path OR content; path hits have no Matches, content hits do
//
// An empty Scope defaults to "content".
func (s *Service) SearchRegex(ctx context.Context, opts RegexOptions) ([]RegexResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultRegexLimit
	}
	maxPerFile := opts.MaxMatchesPerFile
	if maxPerFile <= 0 {
		maxPerFile = defaultMaxMatchesPerFile
	}
	scope := opts.Scope
	if scope == "" {
		scope = "content"
	}

	pattern := opts.Pattern
	if opts.IsGlob {
		pattern = globToRegex(opts.Pattern)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("SearchRegex: invalid pattern %q: %w", opts.Pattern, err)
	}

	var results []RegexResult

	walkErr := s.vault.WalkNotes(ctx, func(rel, abs string) error {
		if len(results) >= limit {
			return filepath.SkipAll
		}

		var result *RegexResult

		// --- path scope ---
		if scope == "path" || scope == "both" {
			if re.MatchString(rel) {
				result = &RegexResult{Path: rel}
				// For path matches we do not add line snippets.
			}
		}

		// --- content scope ---
		if scope == "content" || scope == "both" {
			// Only skip content scan for "both" when we already have a path hit
			// AND we haven't reached the per-file match cap (content matches
			// still add line snippets even when path already matched).
			matches, scanErr := scanFileMatches(ctx, abs, re, maxPerFile)
			if scanErr != nil {
				return scanErr
			}
			if len(matches) > 0 {
				if result == nil {
					result = &RegexResult{Path: rel}
				}
				result.Matches = matches
			}
		}

		if result != nil {
			results = append(results, *result)
		}
		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}

	return results, nil
}

// scanFileMatches opens the file at abs and returns up to maxMatches lines that
// match re. Respects context cancellation between lines.
func scanFileMatches(ctx context.Context, abs string, re *regexp.Regexp, maxMatches int) ([]RegexMatch, error) {
	f, err := os.Open(abs)
	if err != nil {
		// Treat unreadable files as empty rather than a fatal error.
		return nil, nil
	}
	defer f.Close()

	var matches []RegexMatch
	lineNum := 0
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, RegexMatch{
				Line:    lineNum,
				Snippet: strings.TrimSpace(line),
			})
			if len(matches) >= maxMatches {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanFileMatches %q: %w", abs, err)
	}

	return matches, nil
}
