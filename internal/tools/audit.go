package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tylern91/obsidian-mcp-server/internal/response"
	"github.com/tylern91/obsidian-mcp-server/internal/vault"
)

// auditEntry is a single finding in an audit class result.
type auditEntry struct {
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

// defaultAuditClasses is the full set of classes run when none are specified.
var defaultAuditClasses = []string{"orphans", "dangling-links", "untagged", "duplicate-titles"}

func registerAuditNotes(s *server.MCPServer, deps Deps) {
	tool := mcp.NewTool("audit_notes",
		mcp.WithDescription("Audit the vault for hygiene issues: orphans, dangling links, untagged notes, and duplicate titles"),
		mcp.WithString("classes",
			mcp.Description(`JSON array of audit classes to run. Valid values: "orphans", "dangling-links", "untagged", "duplicate-titles". Default: all four.`),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results per class (default: 20)"),
			mcp.DefaultNumber(20),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
	s.AddTool(tool, auditNotesHandler(deps))
}

func auditNotesHandler(deps Deps) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse classes parameter.
		classesStr := req.GetString("classes", "")
		var classes []string
		if classesStr != "" {
			if err := json.Unmarshal([]byte(classesStr), &classes); err != nil {
				return mcp.NewToolResultError("classes: invalid JSON array: " + err.Error()), nil
			}
		}
		if len(classes) == 0 {
			classes = defaultAuditClasses
		}

		// Build a set for quick class membership check.
		wantClass := make(map[string]bool, len(classes))
		for _, c := range classes {
			wantClass[c] = true
		}

		// Parse limit.
		limit := req.GetInt("limit", 20)
		if limit <= 0 {
			limit = 20
		}

		// ── Single-pass walk ────────────────────────────────────────────────────
		type auditData struct {
			allPaths    map[string]bool     // vault-relative paths that exist
			stemToPath  map[string]string   // lowercase stem → vault-relative path (for link resolution)
			tagsByPath  map[string][]string // path → combined tags
			linksByPath map[string][]string // path → link targets (as extracted)
			notesByStem map[string][]string // filename stem → []paths
		}

		ad := auditData{
			allPaths:    make(map[string]bool),
			stemToPath:  make(map[string]string),
			tagsByPath:  make(map[string][]string),
			linksByPath: make(map[string][]string),
			notesByStem: make(map[string][]string),
		}

		walkErr := deps.Vault.WalkNotes(ctx, func(rel, abs string) error {
			ad.allPaths[rel] = true

			// Build stem→path map for link resolution (case-insensitive, no extension).
			base := filepath.Base(rel)
			ext := filepath.Ext(base)
			stem := strings.ToLower(strings.TrimSuffix(base, ext))
			// Last writer wins for duplicate stems; sufficient for orphan detection.
			ad.stemToPath[stem] = rel

			// notesByStem uses the case-sensitive base name stem as key
			// (Obsidian wikilinks are case-insensitive, but stems for duplicate detection
			// should use the raw basename for display accuracy).
			rawStem := strings.TrimSuffix(base, ext)
			ad.notesByStem[rawStem] = append(ad.notesByStem[rawStem], rel)

			data, readErr := os.ReadFile(abs)
			if readErr != nil {
				return nil // skip unreadable
			}
			content := string(data)

			rawFM, body, hasFM := vault.SplitFrontmatter(content)
			var tags []string
			if hasFM {
				fm, parseErr := vault.ParseFrontmatter(rawFM)
				if parseErr == nil {
					tags = append(tags, vault.ExtractFrontmatterTags(fm)...)
				}
			}
			// Merge inline tags (dedup).
			seen := make(map[string]bool, len(tags))
			for _, t := range tags {
				seen[t] = true
			}
			for _, t := range vault.ExtractInlineTags(body) {
				if !seen[t] {
					seen[t] = true
					tags = append(tags, t)
				}
			}
			ad.tagsByPath[rel] = tags

			// Extract link targets.
			targets := vault.ExtractLinks(content)
			ad.linksByPath[rel] = targets

			return nil
		})
		if walkErr != nil {
			return mcp.NewToolResultError("audit_notes: walk failed: " + walkErr.Error()), nil
		}

		// ── Build incoming-links index ──────────────────────────────────────────
		// incomingCount[path] = number of OTHER notes that link to this path.
		// Resolution uses stemToPath so stem-matched wikilinks map to actual paths.
		incomingCount := make(map[string]int, len(ad.allPaths))
		for src, targets := range ad.linksByPath {
			for _, target := range targets {
				resolved := resolveAuditLink(target, ad.allPaths, ad.stemToPath)
				if resolved != "" && resolved != src {
					incomingCount[resolved]++
				}
			}
		}

		// ── Compute results for each requested class ────────────────────────────
		// capEntries collects up to limit+1 entries from a full slice, then
		// returns (trimmed slice, truncated). Collecting one extra lets us detect
		// whether there were MORE results beyond the limit without under-reporting.
		capEntries := func(all []auditEntry) ([]auditEntry, bool) {
			if len(all) > limit {
				return all[:limit], true
			}
			return all, false
		}

		truncated := false

		type resultMap = map[string][]auditEntry

		results := make(resultMap)

		if wantClass["orphans"] {
			var all []auditEntry
			for path := range ad.allPaths {
				hasTags := len(ad.tagsByPath[path]) > 0
				hasIncoming := incomingCount[path] > 0
				if !hasTags && !hasIncoming {
					all = append(all, auditEntry{Path: path, Detail: "no tags and no incoming links"})
					if len(all) > limit {
						// Collected one beyond limit — we know there are more; stop early.
						break
					}
				}
			}
			entries, trunc := capEntries(all)
			if trunc {
				truncated = true
			}
			results["orphans"] = entries
		}

		if wantClass["dangling-links"] {
			var all []auditEntry
		outerDangling:
			for src, targets := range ad.linksByPath {
				for _, target := range targets {
					resolved := resolveAuditLink(target, ad.allPaths, ad.stemToPath)
					if resolved == "" {
						// Dangling.
						all = append(all, auditEntry{Path: src, Detail: "links to " + target})
						if len(all) > limit {
							break outerDangling
						}
					}
				}
			}
			entries, trunc := capEntries(all)
			if trunc {
				truncated = true
			}
			results["dangling-links"] = entries
		}

		if wantClass["untagged"] {
			var all []auditEntry
			for path := range ad.allPaths {
				if len(ad.tagsByPath[path]) == 0 {
					all = append(all, auditEntry{Path: path})
					if len(all) > limit {
						break
					}
				}
			}
			entries, trunc := capEntries(all)
			if trunc {
				truncated = true
			}
			results["untagged"] = entries
		}

		if wantClass["duplicate-titles"] {
			var all []auditEntry
		outerDup:
			for stem, paths := range ad.notesByStem {
				if len(paths) < 2 {
					continue
				}
				for _, p := range paths {
					// Build the "shared with" string excluding self.
					var others []string
					for _, other := range paths {
						if other != p {
							others = append(others, other)
						}
					}
					detail := "shares stem '" + stem + "' with: " + strings.Join(others, ", ")
					all = append(all, auditEntry{Path: p, Detail: detail})
					if len(all) > limit {
						break outerDup
					}
				}
			}
			entries, trunc := capEntries(all)
			if trunc {
				truncated = true
			}
			results["duplicate-titles"] = entries
		}

		// ── Build response ──────────────────────────────────────────────────────
		// Use a dynamic map so we only include requested class keys.
		respMap := make(map[string]any, len(classes)+1)
		for _, c := range classes {
			if entries, ok := results[c]; ok {
				respMap[c] = entries
			}
		}
		respMap["truncated"] = truncated

		prettyPrint := deps.PrettyPrint
		out, err := response.FormatJSON(respMap, prettyPrint)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(out), nil
	}
}

// resolveAuditLink checks whether a link target points to any note in the vault.
// It returns the vault-relative path if found, or "" if dangling.
//
// Resolution strategy:
//  1. Exact match: target is already a vault-relative path in allPaths.
//  2. With ".md" extension: target + ".md" is in allPaths.
//  3. Stem match: the lowercase stem of target matches a known note stem via stemToPath.
func resolveAuditLink(target string, allPaths map[string]bool, stemToPath map[string]string) string {
	// Clean target.
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}

	// 1. Exact match.
	if allPaths[target] {
		return target
	}

	// 2. With .md extension.
	withMD := target + ".md"
	if allPaths[withMD] {
		return withMD
	}

	// 3. Stem match (case-insensitive).
	// Extract stem from target: strip directory prefix and extension.
	base := filepath.Base(target)
	ext := filepath.Ext(base)
	stem := strings.ToLower(strings.TrimSuffix(base, ext))

	if path, ok := stemToPath[stem]; ok {
		// Return the actual vault-relative path so incomingCount is keyed correctly.
		return path
	}

	return ""
}
