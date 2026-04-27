package resources

import "github.com/mark3labs/mcp-go/server"

var VaultStatsHandler = func(deps Deps) server.ResourceHandlerFunc { return vaultStatsHandler(deps) }
var VaultTagsHandler = func(deps Deps) server.ResourceHandlerFunc { return vaultTagsHandler(deps) }
var NoteResourceHandler = func(deps Deps) server.ResourceTemplateHandlerFunc { return noteResourceHandler(deps) }
var PeriodicResourceHandler = func(deps Deps) server.ResourceTemplateHandlerFunc { return periodicResourceHandler(deps) }
var BacklinksResourceHandler = func(deps Deps) server.ResourceTemplateHandlerFunc { return backlinksResourceHandler(deps) }
var PathFromURI = pathFromURI
