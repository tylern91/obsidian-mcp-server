package prompts

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWeeklyReview(s *server.MCPServer, deps Deps) {
	prompt := mcp.NewPrompt("weekly_review",
		mcp.WithPromptDescription("Produce a weekly retrospective from the last 7 daily notes: themes, completed work, unfinished items, suggestions for next week."),
		mcp.WithArgument("weekOffset",
			mcp.ArgumentDescription("Week offset from current week (0 = this week, -1 = last week). Default: 0"),
		),
	)
	s.AddPrompt(prompt, weeklyReviewHandler(deps))
}

func weeklyReviewHandler(deps Deps) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		weekOffset := 0
		if raw := req.Params.Arguments["weekOffset"]; raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return errorPrompt(fmt.Sprintf("invalid weekOffset %q: must be an integer", raw)), nil
			}
			weekOffset = n
		}

		// Collect the 7 daily note dates for the target week.
		// RecentDates(daily, 7) returns last 7 days from today; apply weekOffset shift.
		dates, err := deps.Periodic.RecentDates("daily", 7+weekOffset*-7)
		if err != nil {
			return errorPrompt(fmt.Sprintf("could not resolve daily dates: %v", err)), nil
		}
		// Take only the 7 most relevant dates (shifted window).
		start := weekOffset * -7
		if start < 0 {
			start = 0
		}
		window := dates
		if len(window) > 7 {
			window = window[:7]
		}

		var sb strings.Builder
		sb.WriteString("You are producing a weekly retrospective from an Obsidian vault's daily notes.\n\n")

		found := 0
		for _, d := range window {
			dayPath, err := deps.Periodic.Resolve("daily", int(time.Since(d).Hours()/-24))
			if err != nil {
				continue
			}
			note, err := deps.Vault.ReadNote(ctx, dayPath)
			if err != nil {
				continue
			}
			found++
			sb.WriteString(fmt.Sprintf("## %s (%s)\n%s\n\n", d.Format("Monday, Jan 2"), note.Path, note.Content))
		}

		if found == 0 {
			sb.WriteString("No daily notes found for this week.\n\n")
		}

		sb.WriteString(`Please write a weekly retrospective covering:
1. Major themes and topics that recurred this week
2. Completed work and accomplishments
3. Unfinished items or TODOs that should carry into next week
4. Patterns worth tracking (recurring blockers, mood, energy, focus areas)
5. 2–3 concrete suggestions for next week`)

		return mcp.NewGetPromptResult(
			"Weekly retrospective from daily notes",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(sb.String())),
			},
		), nil
	}
}
