package prompts

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

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
		if weekOffset > 0 {
			return errorPrompt(fmt.Sprintf("invalid weekOffset %d: must be 0 (this week) or negative (weeks ago)", weekOffset)), nil
		}

		// shiftDays is how many days back the target week's most recent day sits
		// relative to today; it's always >= 0 since weekOffset <= 0.
		shiftDays := -weekOffset * 7

		// RecentDates returns shiftDays+7 dates ending at today, descending
		// (dates[0] = today); the target week is the last 7 of those.
		dates, err := deps.Periodic.RecentDates("daily", shiftDays+7)
		if err != nil {
			return errorPrompt(fmt.Sprintf("could not resolve daily dates: %v", err)), nil
		}
		window := dates[shiftDays:]

		var sb strings.Builder
		sb.WriteString("You are producing a weekly retrospective from an Obsidian vault's daily notes.\n\n")

		found := 0
		for i, d := range window {
			// offset is relative to today and must match the date d being labeled.
			offset := -(shiftDays + i)
			dayPath, err := deps.Periodic.Resolve("daily", offset)
			if err != nil {
				slog.Warn("weekly_review: resolve daily path failed", "offset", offset, "err", err)
				continue
			}
			note, err := deps.Vault.ReadNote(ctx, dayPath)
			if err != nil {
				slog.Warn("weekly_review: read daily note failed", "path", dayPath, "err", err)
				continue
			}
			found++
			fmt.Fprintf(&sb, "## %s (%s)\n%s\n\n", d.Format("Monday, Jan 2"), note.Path, note.Content)
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

		return singleUserPrompt("Weekly retrospective from daily notes", sb.String()), nil
	}
}
