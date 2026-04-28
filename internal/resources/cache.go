package resources

import (
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// resourceCache is a simple TTL cache for resource handler results.
// It invalidates when either the TTL expires or the vault root directory
// mtime changes (so tests that write notes see fresh results immediately).
type resourceCache struct {
	mu         sync.Mutex
	data       []mcp.ResourceContents
	rootMtime  time.Time
	computed   time.Time
	ttl        time.Duration
}

// get returns cached data if it is still fresh, otherwise calls recompute,
// stores the result, and returns it.
//
// Freshness requires both:
//   - age < ttl
//   - the vault root directory mtime is unchanged
func (c *resourceCache) get(root string, recompute func() ([]mcp.ResourceContents, error)) ([]mcp.ResourceContents, error) {
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
		currentMtime.Equal(c.rootMtime) {
		return c.data, nil
	}

	result, err := recompute()
	if err != nil {
		return result, err
	}

	c.data = result
	c.rootMtime = currentMtime
	c.computed = now
	return result, nil
}
