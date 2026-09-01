package vault

import (
	"context"
	"os"
	"strings"
)

// HeadingInfo is one heading found by GetNoteOutline.
type HeadingInfo struct {
	Level int    // number of leading '#' characters
	Text  string // heading text, with the '#' prefix and surrounding space trimmed
	Line  int    // 1-indexed line number within the note
}

// GetNoteOutline returns a note's heading tree — level, text, and line number
// — without its body. Built on the same heading parser PatchNote uses
// (headingLevel in mutators.go), not a second one, so the two never disagree
// about what counts as a heading.
func (s *Service) GetNoteOutline(ctx context.Context, path string) ([]HeadingInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "outline", Path: path, Err: err}
	}

	absPath, err := s.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, _, err := readNoteBytes(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &PathError{Op: "outline", Path: path, Err: ErrNotFound}
		}
		return nil, &PathError{Op: "outline", Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")
	headings := make([]HeadingInfo, 0)
	for i, line := range lines {
		prefix := headingLevel(line)
		if prefix == "" {
			continue
		}
		headings = append(headings, HeadingInfo{
			Level: len(prefix),
			Text:  strings.TrimSpace(line[len(prefix):]),
			Line:  i + 1,
		})
	}
	return headings, nil
}

// NoteLines is the result of a bounded line-range read.
type NoteLines struct {
	Path       string // relative path from vault root
	StartLine  int    // 1-indexed, the first line actually returned
	EndLine    int    // 1-indexed, the last line actually returned (inclusive)
	TotalLines int    // total lines in the note
	Content    string // the requested lines, joined by "\n"
}

// ReadNoteLines returns up to lineCount lines starting at startLine
// (1-indexed, inclusive). startLine is clamped to 1; lineCount is clamped to
// 1. A startLine beyond the note's end is not an error — it returns an empty
// Content with EndLine < StartLine, which callers can detect by comparing
// against TotalLines. Bounding lineCount to a sane per-call maximum is the
// tools layer's job (mirroring effectiveLimit elsewhere), not this method's —
// vault.Service takes the range as given.
func (s *Service) ReadNoteLines(ctx context.Context, path string, startLine, lineCount int) (*NoteLines, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "read-lines", Path: path, Err: err}
	}
	if startLine < 1 {
		startLine = 1
	}
	if lineCount < 1 {
		lineCount = 1
	}

	absPath, err := s.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	data, _, err := readNoteBytes(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &PathError{Op: "read-lines", Path: path, Err: ErrNotFound}
		}
		return nil, &PathError{Op: "read-lines", Path: path, Err: err}
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	if startLine > total {
		return &NoteLines{Path: path, StartLine: startLine, EndLine: startLine - 1, TotalLines: total}, nil
	}

	end := startLine + lineCount - 1
	if end > total {
		end = total
	}

	return &NoteLines{
		Path:       path,
		StartLine:  startLine,
		EndLine:    end,
		TotalLines: total,
		Content:    strings.Join(lines[startLine-1:end], "\n"),
	}, nil
}
