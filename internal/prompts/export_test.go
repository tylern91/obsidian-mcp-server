package prompts

import "github.com/mark3labs/mcp-go/server"

var SummarizeNoteHandler = func(deps Deps) server.PromptHandlerFunc { return summarizeNoteHandler(deps) }
var DailyNoteReviewHandler = func(deps Deps) server.PromptHandlerFunc { return dailyNoteReviewHandler(deps) }
var WeeklyReviewHandler = func(deps Deps) server.PromptHandlerFunc { return weeklyReviewHandler(deps) }
var FindRelatedHandler = func(deps Deps) server.PromptHandlerFunc { return findRelatedHandler(deps) }
var VaultHealthCheckHandler = func(deps Deps) server.PromptHandlerFunc { return vaultHealthCheckHandler(deps) }
