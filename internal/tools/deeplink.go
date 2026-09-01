package tools

import (
	"net/url"
	"path/filepath"
	"strings"
)

// obsidianDeepLink builds an obsidian://open deep link to a vault-relative
// note path. Obsidian's file parameter is the path without its extension.
// vaultName empty returns "" — deep links are only meaningful once a vault
// name is configured (see --vault-name / OBSIDIAN_VAULT_NAME).
func obsidianDeepLink(vaultName, relPath string) string {
	if vaultName == "" {
		return ""
	}
	stem := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	q := url.Values{}
	q.Set("vault", vaultName)
	q.Set("file", stem)
	return "obsidian://open?" + q.Encode()
}
