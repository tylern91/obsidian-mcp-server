package search

import (
	"context"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// maxBM25Docs caps the number of notes cached in memory during a single
// SearchBM25 call, to bound memory growth (each doc retains its full raw
// content for snippet extraction) on very large vaults.
const maxBM25Docs = 10_000

// BM25 Okapi parameters.
const (
	bm25K1 = 1.2
	bm25B  = 0.75

	defaultBM25Limit      = 20
	defaultBM25MaxMatches = 3

	// phraseKeySep is the separator used in phrase bigram keys; chosen to
	// avoid collision with real token characters (letters/digits only).
	phraseKeySep = "\x00"
)

// BM25Options controls behaviour of SearchBM25.
type BM25Options struct {
	// Query is the full-text search query. Multi-word queries use OR semantics.
	Query string

	// Limit caps the number of results returned. 0 uses default (20).
	Limit int

	// MaxResults, when > 0, is an upper ceiling applied to the effective
	// limit regardless of Limit or the default. 0 means uncapped.
	MaxResults int

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

// MatchesPathScope reports whether rel matches the given glob scope.
// An empty scope always matches. Exported for reuse in tool handlers.
func MatchesPathScope(rel, scope string) bool {
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

	// Convert the glob to a regex for richer matching (handles ** etc).
	reStr := globToRegex(scope)
	re, err := regexp.Compile(reStr)
	if err != nil {
		return false
	}
	return re.MatchString(rel)
}

// isPhraseKey reports whether term is a phrase-bigram key produced by
// buildTerms, rather than a literal individual token.
func isPhraseKey(term string) bool {
	return strings.Contains(term, phraseKeySep)
}

// pathScopeMatcher precompiles a PathScope glob once so repeated matching
// (once per note during a WalkNotes callback) doesn't recompile the regex
// fallback on every call.
type pathScopeMatcher struct {
	scope string
	re    *regexp.Regexp // nil when scope is empty or invalid
}

func newPathScopeMatcher(scope string) *pathScopeMatcher {
	m := &pathScopeMatcher{scope: scope}
	if scope == "" {
		return m
	}
	if re, err := regexp.Compile(globToRegex(scope)); err == nil {
		m.re = re
	}
	return m
}

func (m *pathScopeMatcher) matches(rel string) bool {
	if m.scope == "" {
		return true
	}
	rel = filepath.ToSlash(rel)
	if matched, err := filepath.Match(m.scope, rel); err == nil && matched {
		return true
	}
	if m.re == nil {
		return false
	}
	return m.re.MatchString(rel)
}

// buildTerms tokenizes the query into individual terms plus, for 2-token
// queries, a consecutive-bigram phrase key ("term0\x00term1") that is scored
// separately in BM25 to reward tight co-occurrence.
// Returns the canonical form (lowercased when CaseSensitive=false).
func buildTerms(query string, caseSensitive bool) []string {
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	raw := markdown.Tokenize(query)
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
	// Phrase bonus: for queries with at least 2 tokens, add a bigram phrase
	// key that is only counted when the first two tokens appear consecutively
	// in the document.  The \x00 separator cannot appear in real tokens
	// (which contain only letters and digits after Tokenize).
	if len(raw) >= 2 {
		phraseKey := raw[0] + phraseKeySep + raw[1]
		if !seen[phraseKey] {
			terms = append(terms, phraseKey)
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
	rel        string
	abs        string
	rawContent string   // full raw file content (avoids a second ReadFile in Pass 2)
	fmText     string   // raw frontmatter text (YAML values flattened)
	bodyText   string   // body after stripping code fences
	allTokens  []string // combined tokenisation used for length
	termFreq   map[string]int
	hasTitle   bool   // true when any query term appears in the filename stem
	titleStem  string // lowercased filename without extension
}

// flattenFrontmatter extracts a single string of all YAML leaf values from raw
// frontmatter (without the --- delimiters) using the vault's YAML parser.
func flattenFrontmatter(raw string) string {
	fm, err := vault.ParseFrontmatter(raw)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	flattenAny(fm, &sb)
	return sb.String()
}

// flattenAny recursively walks a parsed YAML value and writes all leaf
// strings. map[string]any keys are visited in sorted order so the flattened
// output (and therefore tokenisation/term frequencies derived from it) is
// deterministic across runs.
func flattenAny(v any, sb *strings.Builder) {
	switch val := v.(type) {
	case string:
		sb.WriteString(val)
		sb.WriteByte(' ')
	case []any:
		for _, item := range val {
			flattenAny(item, sb)
		}
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flattenAny(val[k], sb)
		}
	}
}

// readDoc reads and parses a note into docContent.
// terms must be the output of buildTerms (individual tokens + optional phrase key).
func readDoc(rel, abs string, terms []string, opts BM25Options) (*docContent, error) {
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	content := string(raw)

	fmRaw, body, _ := vault.SplitFrontmatter(content)

	dc := &docContent{
		rel:        rel,
		abs:        abs,
		rawContent: content,
		termFreq:   map[string]int{},
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
		dc.bodyText = markdown.StripCodeFences(body)
	}

	// Combined tokenisation for term frequencies and document length.
	combined := dc.bodyText + " " + dc.fmText
	if !opts.CaseSensitive {
		combined = strings.ToLower(combined)
	}
	dc.allTokens = markdown.Tokenize(combined)

	for _, tok := range dc.allTokens {
		dc.termFreq[tok]++
	}

	// Phrase bigram pass: for the phrase key (if any), count consecutive
	// occurrences of its own encoded token pair in the token list. The pair
	// is derived by splitting the key itself, not from the deduplicated term
	// list — using indivTerms[0:2] here would score the wrong word pair
	// whenever query tokens repeat or terms dedup out of raw order (e.g.
	// query "cat cat dog" encodes phraseKey "cat\x00cat" but indivTerms would
	// be ["cat","dog"], counting "cat dog" instead of "cat cat").
	for _, term := range terms {
		if !isPhraseKey(term) {
			continue
		}
		t0, t1, ok := strings.Cut(term, phraseKeySep)
		if !ok {
			continue
		}
		tokens := dc.allTokens
		for i := 0; i+1 < len(tokens); i++ {
			if tokens[i] == t0 && tokens[i+1] == t1 {
				dc.termFreq[term]++
			}
		}
	}

	// Check title match (skip phrase key).
	for _, term := range terms {
		if isPhraseKey(term) {
			continue
		}
		if strings.Contains(dc.titleStem, term) {
			dc.hasTitle = true
			break
		}
	}

	return dc, nil
}

// SearchBM25 ranks vault notes for Query using the Okapi BM25 algorithm.
//
// Pass 1: walk all notes matching PathScope, collect corpus stats and cache
// parsed document data. Pass 2: score each cached document using BM25 formula.
func (s *Service) SearchBM25(ctx context.Context, opts BM25Options) ([]BM25Result, error) {
	if opts.Query == "" {
		return nil, nil
	}

	// Both explicitly false → nothing to search.
	if !opts.SearchContent && !opts.SearchFrontmatter {
		return nil, nil
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultBM25Limit
	}
	if opts.MaxResults > 0 && limit > opts.MaxResults {
		limit = opts.MaxResults
	}
	maxPerFile := opts.MaxMatchesPerFile
	if maxPerFile <= 0 {
		maxPerFile = defaultBM25MaxMatches
	}

	terms := buildTerms(opts.Query, opts.CaseSensitive)
	if len(terms) == 0 {
		return nil, nil
	}

	// ---- Pass 1: Corpus statistics + cache documents ----
	stats := &corpusStats{
		df: make(map[string]int),
	}
	var totalTokens int
	var docs []*docContent
	scopeMatcher := newPathScopeMatcher(opts.PathScope)

	err := s.vault.WalkNotes(ctx, func(rel, abs string) error {
		if len(docs) >= maxBM25Docs {
			return filepath.SkipAll
		}
		if !scopeMatcher.matches(rel) {
			return nil
		}

		dc, err := readDoc(rel, abs, terms, opts)
		if err != nil {
			slog.Warn("search_notes: read failed", "path", rel, "err", err)
			return nil
		}

		docs = append(docs, dc)
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

	// ---- Pass 2: Scoring (uses cached docs — no second WalkNotes) ----
	var results []BM25Result

	for _, dc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
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
			continue
		}

		// Determine reason by checking individual terms (not phrase key) against
		// body and frontmatter text.
		bodyMatch := opts.SearchContent && anyTermIn(dc.bodyText, terms, opts.CaseSensitive)
		fmMatch := opts.SearchFrontmatter && dc.fmText != "" && anyTermIn(dc.fmText, terms, opts.CaseSensitive)

		var reason string
		switch {
		case bodyMatch && fmMatch:
			reason = "both"
		case fmMatch:
			reason = "frontmatter"
		case bodyMatch:
			reason = "content"
		default:
			reason = "content" // fallback for scored docs that matched via phrase or title
		}

		// Title boost: +50% score when any term appears in the filename stem.
		if dc.hasTitle {
			score += 0.5 * score
			reason = "title"
		}

		// Collect snippet matches using rawContent (no second ReadFile call).
		matches := collectBM25Matches(dc.rawContent, terms, maxPerFile, opts.CaseSensitive)

		results = append(results, BM25Result{
			Path:       dc.rel,
			Score:      score,
			MatchCount: len(matches),
			Matches:    matches,
			TokenCount: response.CountTokens(dc.rawContent),
			Reason:     reason,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// anyTermIn reports whether any non-phrase-key term in terms is a substring
// of text, honouring caseSensitive the same way term matching does elsewhere.
func anyTermIn(text string, terms []string, caseSensitive bool) bool {
	if !caseSensitive {
		text = strings.ToLower(text)
	}
	for _, term := range terms {
		if isPhraseKey(term) {
			continue
		}
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// collectBM25Matches scans content line by line and collects up to maxMatches
// lines that contain any of the given terms. Code-fenced lines are excluded —
// matching is performed against the stripped version but snippets are taken
// from the original lines. Returns BM25Match slice.
func collectBM25Matches(content string, terms []string, maxMatches int, caseSensitive bool) []BM25Match {
	stripped := markdown.StripCodeFences(content)
	origLines := strings.Split(content, "\n")
	strippedLines := strings.Split(stripped, "\n")

	// Align line counts: use the minimum to be safe if lengths diverge.
	n := len(origLines)
	if len(strippedLines) < n {
		n = len(strippedLines)
	}

	var matches []BM25Match
	for i := 0; i < n && len(matches) < maxMatches; i++ {
		checkLine := strippedLines[i]
		if !caseSensitive {
			checkLine = strings.ToLower(checkLine)
		}
		for _, term := range terms {
			// Phrase keys don't appear literally in text — skip them.
			if isPhraseKey(term) {
				continue
			}
			if strings.Contains(checkLine, term) {
				matches = append(matches, BM25Match{
					Line:    i + 1,
					Snippet: strings.TrimSpace(origLines[i]),
					Term:    term,
				})
				break
			}
		}
	}

	return matches
}
