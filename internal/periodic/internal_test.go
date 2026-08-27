package periodic

import "testing"

// TestGranularityNamesMatchDefaultConfig guards against the two granularity
// enumerations (granularityOffset and defaultConfig) drifting apart, since Go
// has no compile-time way to enforce two map literals share the same keys.
func TestGranularityNamesMatchDefaultConfig(t *testing.T) {
	if len(granularityOffset) != len(defaultConfig) {
		t.Fatalf("granularityOffset has %d entries, defaultConfig has %d", len(granularityOffset), len(defaultConfig))
	}
	for name := range defaultConfig {
		if _, ok := granularityOffset[name]; !ok {
			t.Errorf("defaultConfig has granularity %q with no matching entry in granularityOffset", name)
		}
	}
	for name := range granularityOffset {
		if _, ok := defaultConfig[name]; !ok {
			t.Errorf("granularityOffset has granularity %q with no matching entry in defaultConfig", name)
		}
	}
}
