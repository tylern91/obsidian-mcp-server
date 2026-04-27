package search

import (
	"github.com/tylern91/obsidian-mcp-server/internal/markdown"
)

// StripCodeFences returns the content with all fenced code blocks and inline
// code spans replaced by a single space.
//
// This is a thin wrapper around markdown.StripCodeFences so that existing
// callers inside the search package continue to compile without change.
func StripCodeFences(content string) string {
	return markdown.StripCodeFences(content)
}

// Tokenize splits text into lowercase word tokens.
//
// This is a thin wrapper around markdown.Tokenize so that existing callers
// inside the search package continue to compile without change.
func Tokenize(text string) []string {
	return markdown.Tokenize(text)
}
