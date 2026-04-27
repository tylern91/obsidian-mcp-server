package search

import (
	"bufio"
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// BM25 Okapi parameters.
const (
	bm25K1 = 1.2
	bm25B  = 0.75

	defaultBM25Limit        = 20
	defaultBM25MaxMatches   = 3
)

// BM25Options controls behaviour of SearchBM25.
type BM25Options struct {
	// Query is the full-text search query. Multi-word queries use OR semantics.
	Query string

	// Limit caps the number of results returned. 0 uses default (20).
	Limit int

	// MaxMatchesPerFile caps snippet lines collected per file. 0 uses default (3).
	MaxMatchesPerFile int

	// CaseSensitive, when true, does not lowercase query terms or document text.
	CaseSensitive bool

	// SearchContent controls whether note body content is scored. Defaults to true.
	// At least one of SearchContent or SearchFrontmatter must be true.
	SearchContent bool

	// SearchFrontmatter controls whether YAML frontmatter is scored. Defaults to true.
	SearchFrontmatter bool

	// PathScope is a filepath.Match-style glob to restrict which notes are scored.
	// Empty string means all notes.
	PathScope string
}

// BM25Match represents a single matching line within a file.
type BM25Match struct {
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Term    string `json:"term"` // which query term matched this line
}

// BM25Result holds the BM25 ranking result for a single vault note.
type BM25Result struct {
	Path       string      `json:"path"`
	Score      float64     `json:"score"`
	MatchCount int         `json:"matchCount"`
	Matches    []BM25Match `json:"matches"`
	TokenCount int         `json:"tokenCount"`
	// Reason is "title" | "content" | "frontmatter" | "both"
	Reason string `json:"reason"`
}

// matchesPathScope reports whether rel matches the given glob scope.
// An empty scope always matches.
func matchesPathScope(rel, scope string) bool {
	if scope == "" {
		return true
	}
	// Normalise to forward slashes for consistency.
	rel = filepath.ToSlash(rel)
	// Try filepath.Match directly.
	matched, err := filepath.Match(scope, rel)
	if err != nil {
		return false
	}
	if matched {
		return true
	}
	// Also try matching the basename alone (supports globs like "Search/*").
	base := filepath.Base(rel)
	dir := filepath.Dir(rel)
	// Re-assemble and try the original pattern against the full rel path.
	_ = base
	_ = dir

	// Convert the glob to a regex for richer matching (handles ** etc).
	reStr := globToRegex(scope)
	re, err := regexp.Compile(reStr)
	if err != nil {
		return false
	}
	return re.MatchString(rel)
}

// buildTerms tokenizes the query into individual terms plus, for multi-word
// queries, a concatenated phrase term (bonus for exact adjacency).
// Returns the canonical form (lowercased when CaseSensitive=false).
func buildTerms(query string, caseSensitive bool) []string {
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	raw := Tokenize(query)
	if len(raw) == 0 {
		return nil
	}
	terms := make([]string, 0, len(raw)+1)
	seen := map[string]bool{}
	for _, t := range raw {
		if !seen[t] {
			seen[t] = true
			terms = append(terms, t)
		}
	}
	// Phrase bonus: join all terms (strips spaces) — rewards tight co-occurrence.
	if len(raw) > 1 {
		phrase := strings.Join(raw, "")
		if !seen[phrase] {
			terms = append(terms, phrase)
		}
	}
	return terms
}

// corpusStats holds aggregate statistics collected in the first pass.
type corpusStats struct {
	N     int            // total document count
	avgDL float64        // average document length in tokens
	df    map[string]int // document frequency per term
}

// docContent holds the parsed content of a single note.
type docContent struct {
	rel         string
	abs         string
	fmText      string   // raw frontmatter text (YAML values flattened)
	bodyText    string   // body after stripping code fences
	allTokens   []string // combined tokenisation used for length
	termFreq    map[string]int
	hasTitle    bool   // true when any query term appears in the filename stem
	titleStem   string // lowercased filename without extension
}

// flattenFrontmatter extracts a single string of all YAML values from raw
// frontmatter (without the --- delimiters). It is intentionally simple.
func flattenFrontmatter(raw string) string {
	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		// YAML key: value — grab everything after the first colon.
		if idx := strings.Index(line, ":"); idx >= 0 {
			val := strings.TrimSpace(line[idx+1:])
			// Strip YAML list brackets [ ... ]
			val = strings.Trim(val, "[]")
			if val != "" {
				sb.WriteString(val)
				sb.WriteByte(' ')
			}
		}
	}
	return sb.String()
}

// readDoc reads and parses a note into docContent.
func readDoc(rel, abs string, terms []string, opts BM25Options) (*docContent, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil // treat unreadable as missing
	}
	content := string(raw)

	fmRaw, body, _ := vault.SplitFrontmatter(content)

	dc := &docContent{
		rel:      rel,
		abs:      abs,
		termFreq: map[string]int{},
	}

	// Derive title stem.
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	dc.titleStem = strings.ToLower(strings.TrimSuffix(base, ext))

	// Frontmatter text.
	if opts.SearchFrontmatter && fmRaw != "" {
		dc.fmText = flattenFrontmatter(fmRaw)
	}

	// Body text (strip code fences).
	if opts.SearchContent {
		dc.bodyText = StripCodeFences(body)
	}

	// Combined tokenisation for term frequencies and document length.
	combined := dc.bodyText + " " + dc.fmText
	if !opts.CaseSensitive {
		combined = strings.ToLower(combined)
	}
	dc.allTokens = Tokenize(combined)

	// Build term frequency map.
	for _, tok := range dc.allTokens {
		dc.termFreq[tok]++
	}
	// Also count phrase occurrences — join adjacent token pairs as proxy.
	// The phrase term is the concatenation of all query raw tokens.
	// We check for exact adjacent sequence in the token list.
	rawTerms := terms // these are already the canonical (possibly lowercased) individual terms + phrase
	for _, t := range rawTerms {
		// Phrase term (longer than any single word): count overlapping window matches.
		if len(t) > 0 && !strings.ContainsAny(t, " \t") {
			// Already handled above via allTokens
			_ = t
		}
	}

	// Check title match.
	for _, term := range terms {
		if strings.Contains(dc.titleStem, term) {
			dc.hasTitle = true
			break
		}
	}

	return dc, nil
}

// SearchBM25 ranks vault notes for Query using the Okapi BM25 algorithm.
//
// The two-pass design:
//   - Pass 1: walk all notes matching PathScope, collect N, avgDL, df[term]
//   - Pass 2: score each document using BM25 formula, collect snippets
func (s *Service) SearchBM25(ctx context.Context, opts BM25Options) ([]BM25Result, error) {
	if opts.Query == "" {
		return nil, nil
	}

	// Default flags: both SearchContent and SearchFrontmatter default to true.
	if !opts.SearchContent && !opts.SearchFrontmatter {
		// Both explicitly false → nothing to search.
		return nil, nil
	}
	// Apply default true behaviour when both are zero-valued (false in Go).
	// Callers that want only one must set the other to false explicitly.
	// However, since Go zero-value is false, we need a different convention.
	// Per spec: "SearchContent bool  // default true".
	// We treat both being false as "use defaults" only if the query is non-empty.
	// Actually the spec says "if both false, no search" — so keep as-is but
	// the caller is expected to set at least one. The tool handler will set defaults.
	//
	// For the BM25 function itself: if both are false we already returned above.
	// But callers wanting both enabled should pass SearchContent=true, SearchFrontmatter=true.
	// The zero-value issue means callers must be explicit. Document this in the tool layer.

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultBM25Limit
	}
	maxPerFile := opts.MaxMatchesPerFile
	if maxPerFile <= 0 {
		maxPerFile = defaultBM25MaxMatches
	}

	terms := buildTerms(opts.Query, opts.CaseSensitive)
	if len(terms) == 0 {
		return nil, nil
	}

	// ---- Pass 1: Corpus statistics ----
	stats := &corpusStats{
		df: make(map[string]int),
	}
	var totalTokens int

	err := s.vault.WalkNotes(ctx, func(rel, abs string) error {
		if !matchesPathScope(rel, opts.PathScope) {
			return nil
		}

		dc, err := readDoc(rel, abs, terms, opts)
		if err != nil || dc == nil {
			return nil
		}

		stats.N++
		totalTokens += len(dc.allTokens)

		// Count df for each term that appears in this document.
		counted := map[string]bool{}
		for _, term := range terms {
			if !counted[term] && dc.termFreq[term] > 0 {
				stats.df[term]++
				counted[term] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if stats.N == 0 {
		return nil, nil
	}
	stats.avgDL = float64(totalTokens) / float64(stats.N)

	// ---- Pass 2: Scoring ----
	var results []BM25Result

	err = s.vault.WalkNotes(ctx, func(rel, abs string) error {
		if !matchesPathScope(rel, opts.PathScope) {
			return nil
		}

		dc, err := readDoc(rel, abs, terms, opts)
		if err != nil || dc == nil {
			return nil
		}

		docLen := float64(len(dc.allTokens))

		var score float64
		for _, term := range terms {
			tf := float64(dc.termFreq[term])
			if tf == 0 {
				continue
			}
			df := float64(stats.df[term])
			n := float64(stats.N)

			idf := math.Log((n-df+0.5)/(df+0.5) + 1)
			tfNorm := (tf * (bm25K1 + 1)) / (tf + bm25K1*(1-bm25B+bm25B*docLen/stats.avgDL))
			score += idf * tfNorm
		}

		if score == 0 {
			return nil
		}

		// Determine reason.
		reason := "content"
		if opts.SearchFrontmatter && dc.fmText != "" {
			fmLower := dc.fmText
			bodyLower := dc.bodyText
			if !opts.CaseSensitive {
				fmLower = strings.ToLower(fmLower)
				bodyLower = strings.ToLower(bodyLower)
			}
			fmMatch := false
			bodyMatch := false
			for _, term := range terms {
				if strings.Contains(fmLower, term) {
					fmMatch = true
				}
				if strings.Contains(bodyLower, term) {
					bodyMatch = true
				}
			}
			if fmMatch && bodyMatch {
				reason = "both"
			} else if fmMatch {
				reason = "frontmatter"
			}
		}

		// Title boost: +50% score when any term appears in the filename stem.
		if dc.hasTitle {
			score += 0.5 * score
			reason = "title"
		}

		// Collect snippet matches.
		matches := collectBM25Matches(abs, terms, maxPerFile, opts.CaseSensitive)

		fullContent, _ := os.ReadFile(abs)

		results = append(results, BM25Result{
			Path:       rel,
			Score:      score,
			MatchCount: len(matches),
			Matches:    matches,
			TokenCount: response.CountTokens(string(fullContent)),
			Reason:     reason,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit.
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// collectBM25Matches scans the file line by line and collects up to maxMatches
// lines that contain any of the given terms. Returns BM25Match slice.
func collectBM25Matches(abs string, terms []string, maxMatches int, caseSensitive bool) []BM25Match {
	f, err := os.Open(abs)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []BM25Match
	lineNum := 0
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		if len(matches) >= maxMatches {
			break
		}
		lineNum++
		line := scanner.Text()
		checkLine := line
		if !caseSensitive {
			checkLine = strings.ToLower(line)
		}
		for _, term := range terms {
			if strings.Contains(checkLine, term) {
				matches = append(matches, BM25Match{
					Line:    lineNum,
					Snippet: strings.TrimSpace(line),
					Term:    term,
				})
				break
			}
		}
	}

	return matches
}
