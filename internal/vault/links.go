package vault

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// combinedLinkRegex matches Obsidian wikilinks/embeds and Markdown links in one
// pass, preserving document order.
//
//   - Group 1: wikilink target (from [[target]], ![[target]], [[target|alias]], [[target#anchor]])
//   - Group 2: markdown link path (from [text](path.md))
var combinedLinkRegex = regexp.MustCompile(
	`!?\[\[([^\]|#]+?)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]` +
		`|` +
		`\[[^\]]*\]\(([^)]+\.md)\)`,
)

// ExtractLinks returns all unique link targets from content in document order,
// combining wikilinks (including embeds) and Markdown links.
//
// Anchors and aliases are stripped from wikilinks; only the target path or name
// is kept.
func ExtractLinks(content string) []string {
	seen := make(map[string]struct{})
	var out []string

	for _, m := range combinedLinkRegex.FindAllStringSubmatch(content, -1) {
		var target string
		if m[1] != "" {
			target = strings.TrimSpace(m[1])
		} else if m[2] != "" {
			target = strings.TrimSpace(m[2])
		}
		if target == "" {
			continue
		}
		if _, dup := seen[target]; !dup {
			seen[target] = struct{}{}
			out = append(out, target)
		}
	}

	return out
}

// Backlink records a note that links to a target, with the line number and a
// trimmed excerpt of the linking line.
type Backlink struct {
	Path    string // relative vault path of the note containing the link
	Line    int    // 1-indexed line number of the linking line
	Snippet string // trimmed content of the linking line
}

// GetBacklinks finds all notes in the vault that contain a link to the note at
// targetPath.
//
// Matching is by basename without extension (e.g. "simple" for "Notes/simple.md")
// OR by relative path with or without the ".md" extension.  Both wikilinks and
// Markdown links are checked.  The target note itself is excluded from results.
//
// The method is read-only and does not hold the service mutex.
func (s *Service) GetBacklinks(ctx context.Context, targetPath string) ([]Backlink, error) {
	_, absTarget, err := s.sanitizePath("get_backlinks", targetPath)
	if err != nil {
		return nil, err
	}

	if _, statErr := os.Stat(absTarget); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, &PathError{Op: "get_backlinks", Path: targetPath, Err: ErrNotFound}
		}
		return nil, &PathError{Op: "get_backlinks", Path: targetPath, Err: statErr}
	}

	// Build the set of strings a link must equal to be considered a match.
	relTarget := filepath.ToSlash(targetPath)
	base := filepath.Base(relTarget)
	baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	relNoExt := strings.TrimSuffix(relTarget, filepath.Ext(relTarget))

	matchTargets := map[string]bool{
		baseNoExt: true,
		base:      true,
		relNoExt:  true,
		relTarget: true,
	}

	var results []Backlink

	err = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		if d.IsDir() {
			rel, relErr := filepath.Rel(s.root, path)
			if relErr != nil {
				return nil
			}
			if rel != "." && s.filter != nil && s.filter.IsIgnored(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(s.root, path)
		if relErr != nil {
			return nil
		}

		if s.filter != nil {
			if s.filter.IsIgnored(rel) {
				return nil
			}
			if !s.filter.IsAllowedExtension(filepath.Ext(path)) {
				return nil
			}
		}

		if filepath.Clean(path) == filepath.Clean(absTarget) {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			for _, m := range combinedLinkRegex.FindAllStringSubmatch(line, -1) {
				var lt string
				if m[1] != "" {
					lt = strings.TrimSpace(m[1])
				} else if m[2] != "" {
					lt = strings.TrimSpace(m[2])
				}
				if lt == "" {
					continue
				}
				ltSlash := filepath.ToSlash(lt)
				ltNoExt := strings.TrimSuffix(ltSlash, filepath.Ext(ltSlash))
				if matchTargets[ltSlash] || matchTargets[ltNoExt] {
					results = append(results, Backlink{
						Path:    filepath.ToSlash(rel),
						Line:    lineNum,
						Snippet: strings.TrimSpace(line),
					})
					break
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}
