package response

import (
	"encoding/json"
	"fmt"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

var (
	encoder     *tiktoken.Tiktoken
	encoderOnce sync.Once
)

func initEncoder() {
	encoderOnce.Do(func() {
		enc, err := tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			// The BPE ranks are embedded via go:embed (tiktoken_loader.go), so this
			// path only fails on a corrupted binary — not a condition to recover from.
			panic(fmt.Sprintf("response: failed to load embedded cl100k_base encoding: %v", err))
		}
		encoder = enc
	})
}

// CountTokens returns the number of tokens in text using the cl100k_base encoding
// (used by gpt-4o and GPT-4).
func CountTokens(text string) int {
	initEncoder()
	return len(encoder.Encode(text, nil, nil))
}

// FormatJSON serializes data to a JSON string. If prettyPrint is true, the output
// is indented with two spaces per level.
func FormatJSON(data any, prettyPrint bool) (string, error) {
	var b []byte
	var err error
	if prettyPrint {
		b, err = json.MarshalIndent(data, "", "  ")
	} else {
		b, err = json.Marshal(data)
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// HeadRunes returns up to the first n runes of s.
func HeadRunes(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if n >= len(r) {
		return s
	}
	return string(r[:n])
}
