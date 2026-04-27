package prompts

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerVaultHealthCheck(s *server.MCPServer, deps Deps) {
	prompt := mcp.NewPrompt("vault_health_check",
		mcp.WithPromptDescription("Audit the vault for hygiene issues and ask the AI to prioritize fixes: orphaned notes, dangling links, untagged notes, duplicate titles."),
	)
	s.AddPrompt(prompt, vaultHealthCheckHandler(deps))
}

func vaultHealthCheckHandler(deps Deps) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		report, err := buildHealthReport(ctx, deps)
		if err != nil {
			return errorPrompt(fmt.Sprintf("vault walk failed: %v", err)), nil
		}

		text := report + `

Based on the audit above, please:
1. Prioritize the top 10 most actionable hygiene fixes (explain why each matters)
2. Identify which dangling links are likely typos vs. intentional placeholders
3. Suggest tag groupings for the untagged notes based on their filenames/paths
4. Flag any duplicate titles that are likely accidental vs. intentional variations`

		return mcp.NewGetPromptResult(
			"Vault hygiene audit and fix priorities",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text)),
			},
		), nil
	}
}

func buildHealthReport(ctx context.Context, deps Deps) (string, error) {
	type noteData struct {
		tags     []string
		links    []string // outgoing wikilink targets
		incoming int      // populated after walk
	}

	all := make(map[string]*noteData)  // rel path → data
	stems := make(map[string][]string) // stem → []rel paths (for duplicate detection)

	err := deps.Vault.WalkNotes(ctx, func(rel, _ string) error {
		note, err := deps.Vault.ReadNote(ctx, rel)
		if err != nil {
			return nil // skip unreadable notes
		}
		tags, _ := deps.Vault.ListTags(ctx, rel)
		links := extractWikilinks(note.Content)
		all[rel] = &noteData{tags: tags, links: links}

		stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		stems[stem] = append(stems[stem], rel)
		return nil
	})
	if err != nil {
		return "", err
	}

	// Build stem→path index for dangling-link detection.
	stemToPath := make(map[string]string)
	for rel := range all {
		stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		stemToPath[strings.ToLower(stem)] = rel
	}

	// Count incoming links per note.
	for _, nd := range all {
		for _, target := range nd.links {
			if resolved, ok := stemToPath[strings.ToLower(target)]; ok {
				all[resolved].incoming++
			}
		}
	}

	var orphans, untagged, dangling, dupes []string

	for rel, nd := range all {
		noTags := len(nd.tags) == 0
		noIncoming := nd.incoming == 0
		if noTags && noIncoming {
			orphans = append(orphans, rel)
		}
		if noTags {
			untagged = append(untagged, rel)
		}
		for _, target := range nd.links {
			if _, ok := stemToPath[strings.ToLower(target)]; !ok {
				dangling = append(dangling, fmt.Sprintf("%s → [[%s]]", rel, target))
			}
		}
	}
	for stem, paths := range stems {
		if len(paths) > 1 {
			dupes = append(dupes, fmt.Sprintf("%s: %s", stem, strings.Join(paths, ", ")))
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Vault health report (%d notes total)\n\n", len(all)))

	cap10 := func(label string, items []string) {
		sb.WriteString(fmt.Sprintf("### %s (%d)\n", label, len(items)))
		for i, item := range items {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(items)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("  - %s\n", item))
		}
		sb.WriteString("\n")
	}

	cap10("Orphaned notes (no tags, no incoming links)", orphans)
	cap10("Dangling links (target not found)", dangling)
	cap10("Untagged notes", untagged)
	cap10("Duplicate titles (same filename stem)", dupes)

	return sb.String(), nil
}
