package tools

import "github.com/mark3labs/mcp-go/server"

var ReadNoteHandler = func(deps Deps) server.ToolHandlerFunc { return readNoteHandler(deps) }
var WriteNoteHandler = func(deps Deps) server.ToolHandlerFunc { return writeNoteHandler(deps) }
var ListDirectoryHandler = func(deps Deps) server.ToolHandlerFunc { return listDirectoryHandler(deps) }
var GetFrontmatterHandler = func(deps Deps) server.ToolHandlerFunc { return getFrontmatterHandler(deps) }
var UpdateFrontmatterHandler = func(deps Deps) server.ToolHandlerFunc { return updateFrontmatterHandler(deps) }
var ManageTagsHandler = func(deps Deps) server.ToolHandlerFunc { return manageTagsHandler(deps) }
var ListAllTagsHandler = func(deps Deps) server.ToolHandlerFunc { return listAllTagsHandler(deps) }
var GetBacklinksHandler = func(deps Deps) server.ToolHandlerFunc { return getBacklinksHandler(deps) }
var PatchNoteHandler = func(deps Deps) server.ToolHandlerFunc { return patchNoteHandler(deps) }
var DeleteNoteHandler = func(deps Deps) server.ToolHandlerFunc { return deleteNoteHandler(deps) }
var MoveNoteHandler = func(deps Deps) server.ToolHandlerFunc { return moveNoteHandler(deps) }
var SearchNotesHandler = func(deps Deps) server.ToolHandlerFunc { return searchNotesHandler(deps) }
var SearchRegexHandler = func(deps Deps) server.ToolHandlerFunc { return searchRegexHandler(deps) }
var ReadMultipleNotesHandler = func(deps Deps) server.ToolHandlerFunc { return readMultipleNotesHandler(deps) }
var GetNotesInfoHandler = func(deps Deps) server.ToolHandlerFunc { return getNotesInfoHandler(deps) }
var VaultStatsHandler = func(deps Deps) server.ToolHandlerFunc { return getVaultStatsHandler(deps) }
var RecentChangesHandler = func(deps Deps) server.ToolHandlerFunc { return recentChangesHandler(deps) }
var GetPeriodicNoteHandler = func(deps Deps) server.ToolHandlerFunc { return getPeriodicNoteHandler(deps) }
var GetRecentPeriodicNotesHandler = func(deps Deps) server.ToolHandlerFunc {
	return getRecentPeriodicNotesHandler(deps)
}
