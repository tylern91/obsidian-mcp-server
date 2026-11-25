package resources

import (
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// resourceCache is a simple TTL cache for resource handler results.
// It invalidates when the TTL expires, the vault root directory mtime changes
// (external adds and removes at the top level), or the vault's mutation
// generation moves (any write, move or delete, including nested notes whose
// edits change no root mtime).
type resourceCache struct {
	mu        sync.Mutex
	data      []mcp.ResourceContents
	rootMtime time.Time
	gen       uint64
	computed  time.Time
	ttl       time.Duration
}

// get returns cached data if it is still fresh, otherwise calls recompute,
// stores the result, and returns it.
//
// Freshness requires both:
//   - age < ttl
//   - the vault root directory mtime is unchanged
//   - gen, read before recompute so a concurrent write is never masked, is unchanged
func (c *resourceCache) get(root string, gen uint64, recompute func() ([]mcp.ResourceContents, error)) ([]mcp.ResourceContents, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check vault root mtime so that writes during tests invalidate the cache.
	var currentMtime time.Time
	if info, err := os.Stat(root); err == nil {
		currentMtime = info.ModTime()
	}

	if c.data != nil &&
		now.Sub(c.computed) < c.ttl &&
		currentMtime.Equal(c.rootMtime) &&
		gen == c.gen {
		return c.data, nil
	}

	result, err := recompute()
	if err != nil {
		return result, err
	}

	c.data = result
	c.rootMtime = currentMtime
	c.gen = gen
	c.computed = now
	return result, nil
}
