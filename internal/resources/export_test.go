package resources

import "github.com/mark3labs/mcp-go/server"

var PathFromURI = pathFromURI

// VaultStatsHandler exposes the cached vault-stats resource handler.
func VaultStatsHandler(deps Deps) server.ResourceHandlerFunc {
	return cachedResourceHandler(deps, computeVaultStats)
}

// VaultTagsHandler exposes the cached vault-tags resource handler.
func VaultTagsHandler(deps Deps) server.ResourceHandlerFunc {
	return cachedResourceHandler(deps, computeVaultTags)
}
