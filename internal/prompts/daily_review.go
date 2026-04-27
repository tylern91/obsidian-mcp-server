package prompts

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDailyNoteReview(s *server.MCPServer, deps Deps) {
	prompt := mcp.NewPrompt("daily_note_review",
		mcp.WithPromptDescription("Review a daily note: identify carryover TODOs, suggest links to related notes, flag missing tags."),
		mcp.WithArgument("offset",
			mcp.ArgumentDescription("Day offset from today (0 = today, -1 = yesterday). Default: 0"),
		),
	)
	s.AddPrompt(prompt, dailyNoteReviewHandler(deps))
}

func dailyNoteReviewHandler(deps Deps) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		offset := 0
		if raw := req.Params.Arguments["offset"]; raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return errorPrompt(fmt.Sprintf("invalid offset %q: must be an integer", raw)), nil
			}
			offset = n
		}

		todayPath, err := deps.Periodic.Resolve("daily", offset)
		if err != nil {
			return errorPrompt(fmt.Sprintf("could not resolve daily note: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("You are reviewing daily notes from an Obsidian vault.\n\n")

		today, err := deps.Vault.ReadNote(ctx, todayPath)
		if err != nil {
			sb.WriteString(fmt.Sprintf("Today's note (%s): not found or unreadable.\n\n", todayPath))
		} else {
			sb.WriteString(fmt.Sprintf("Today's note (%s):\n---\n%s\n---\n\n", today.Path, today.Content))
		}

		// Include the previous day for carryover context.
		prevPath, err := deps.Periodic.Resolve("daily", offset-1)
		if err == nil {
			prev, err := deps.Vault.ReadNote(ctx, prevPath)
			if err == nil {
				sb.WriteString(fmt.Sprintf("Previous day's note (%s):\n---\n%s\n---\n\n", prev.Path, prev.Content))
			}
		}

		sb.WriteString(`Please:
1. Identify any TODOs or tasks that appear unfinished and should carry forward
2. Suggest 3–5 existing notes in the vault worth linking from today's note (based on topics mentioned)
3. Flag any missing tags that seem relevant given the content
4. Note any patterns or themes worth tracking over time`)

		return mcp.NewGetPromptResult(
			"Daily note review",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(sb.String())),
			},
		), nil
	}
}
