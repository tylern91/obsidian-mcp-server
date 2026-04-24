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
