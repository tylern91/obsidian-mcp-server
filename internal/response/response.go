package response

import (
	"encoding/json"
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
		if err == nil {
			encoder = enc
		}
		// encoder stays nil on failure; CountTokens falls back to approximation
	})
}

// CountTokens returns the number of tokens in text using the cl100k_base encoding
// (used by gpt-4o and GPT-4). Falls back to len(text)/4 if the encoder is unavailable.
func CountTokens(text string) int {
	initEncoder()
	if encoder == nil {
		return len(text) / 4 // approximation: ~4 chars per token
	}
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

// Truncate returns (s[:maxRunes], true) if s exceeds maxRunes runes,
// otherwise (s, false). The cut is always on a rune boundary.
// CRLF sequences (\r\n) are kept together: if a \n would fall at or before the
// cut point but its preceding \r would be split off, the cut is moved back
// before the \r so the pair travels together.
func Truncate(s string, maxRunes int) (string, bool) {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s, false
	}
	cut := maxRunes
	// Do not split a CRLF pair: if the rune at cut-1 is \r and cut is within
	// bounds with \n next, OR if rune at cut is \n and cut-1 is \r,
	// retract the cut to before the \r.
	if cut > 0 && cut < len(r) && r[cut-1] == '\r' && r[cut] == '\n' {
		// \r is at cut-1, \n is at cut — cutting here would orphan \r
		cut--
	}
	return string(r[:cut]), true
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
