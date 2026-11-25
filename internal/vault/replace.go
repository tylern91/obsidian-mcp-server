package vault

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ReplaceResult is the outcome of ReplaceInNote.
type ReplaceResult struct {
	Path             string
	OccurrencesFound int // total matches in the note before any cap
	Replaced         int // how many were actually replaced (<= OccurrencesFound)
}

// ReplaceInNote replaces occurrences of pattern with replacement in a note's
// content. isRegex selects RE2 regex matching (replacement may use $1-style
// backreferences) vs. literal substring matching. maxOccurrences <= 0 means
// unbounded; otherwise only the first maxOccurrences matches (in document
// order) are replaced, and OccurrencesFound still reports the true total so
// a caller can tell the result was capped. An empty pattern is rejected.
// WithIfMatch makes the replace conditional on the note's current etag.
func (s *Service) ReplaceInNote(ctx context.Context, path, pattern, replacement string, isRegex bool, maxOccurrences int, opts ...WriteOpt) (*ReplaceResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, &PathError{Op: "replace_in_note", Path: path, Err: err}
	}

	if pattern == "" {
		return nil, &PathError{Op: "replace_in_note", Path: path, Err: fmt.Errorf("pattern must not be empty")}
	}

	_, absPath, err := s.sanitizePath("replace_in_note", path)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.existsStrict("replace_in_note", path, absPath); err != nil {
		return nil, err
	}
	if err := s.checkSymlinksForWrite("replace_in_note", path, absPath); err != nil {
		return nil, err
	}

	data, _, err := readNoteBytes(absPath)
	if err != nil {
		return nil, &PathError{Op: "replace_in_note", Path: path, Err: err}
	}
	if err := checkIfMatch("replace_in_note", path, data, applyWriteOpts(opts)); err != nil {
		return nil, err
	}
	content := string(data)

	var re *regexp.Regexp
	if isRegex {
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, &PathError{Op: "replace_in_note", Path: path, Err: fmt.Errorf("invalid regex: %w", err)}
		}
	} else {
		re = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	matches := re.FindAllStringSubmatchIndex(content, -1)
	found := len(matches)
	if found == 0 {
		return &ReplaceResult{Path: path, OccurrencesFound: 0, Replaced: 0}, nil
	}

	limit := found
	if maxOccurrences > 0 && maxOccurrences < found {
		limit = maxOccurrences
	}

	var b strings.Builder
	last := 0
	for i := 0; i < limit; i++ {
		m := matches[i]
		b.WriteString(content[last:m[0]])
		if isRegex {
			b.Write(re.ExpandString(nil, replacement, content, m))
		} else {
			b.WriteString(replacement)
		}
		last = m[1]
	}
	b.WriteString(content[last:])
	newContent := b.String()

	if int64(len(newContent)) > maxFileSizeBytes {
		return nil, &PathError{Op: "replace_in_note", Path: path, Err: ErrFileTooLarge}
	}

	if newContent != content {
		if writeErr := s.writeFileAtomic(absPath, []byte(newContent)); writeErr != nil {
			return nil, &PathError{Op: "replace_in_note", Path: path, Err: writeErr}
		}
	}

	return &ReplaceResult{Path: path, OccurrencesFound: found, Replaced: limit}, nil
}
