package response

import (
	_ "embed"
	"encoding/base64"
	"strconv"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

//go:embed assets/cl100k_base.tiktoken
var cl100kBaseBPE []byte

// embeddedBpeLoader serves the vendored cl100k_base rank table, so token
// counting never reaches openaipublic.blob.core.windows.net.
type embeddedBpeLoader struct{}

func (embeddedBpeLoader) LoadTiktokenBpe(_ string) (map[string]int, error) {
	ranks := make(map[string]int)
	for _, line := range strings.Split(string(cl100kBaseBPE), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		ranks[string(token)] = rank
	}
	return ranks, nil
}

func init() {
	tiktoken.SetBpeLoader(embeddedBpeLoader{})
}
