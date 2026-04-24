package tools

import "github.com/mark3labs/mcp-go/server"

var ReadNoteHandler = func(deps Deps) server.ToolHandlerFunc { return readNoteHandler(deps) }
var WriteNoteHandler = func(deps Deps) server.ToolHandlerFunc { return writeNoteHandler(deps) }
var ListDirectoryHandler = func(deps Deps) server.ToolHandlerFunc { return listDirectoryHandler(deps) }
