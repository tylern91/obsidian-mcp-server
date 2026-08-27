package tools

import (
	"reflect"
	"testing"
)

func TestEffectiveLimit(t *testing.T) {
	tests := []struct {
		name                         string
		requested, fallback, ceiling int
		want                         int
	}{
		{"requested wins over fallback", 5, 10, 0, 5},
		{"zero requested uses fallback", 0, 10, 0, 10},
		{"negative requested uses fallback", -1, 10, 0, 10},
		{"ceiling caps requested", 50, 10, 20, 20},
		{"ceiling caps fallback", 0, 50, 20, 20},
		{"ceiling above value is no-op", 5, 10, 20, 5},
		{"zero ceiling is uncapped", 50, 10, 0, 50},
		{"negative ceiling is uncapped", 50, 10, -1, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveLimit(tt.requested, tt.fallback, tt.ceiling)
			if got != tt.want {
				t.Errorf("effectiveLimit(%d, %d, %d) = %d, want %d", tt.requested, tt.fallback, tt.ceiling, got, tt.want)
			}
		})
	}
}

func TestClampBatch(t *testing.T) {
	paths := []string{"a", "b", "c"}

	got, truncated := clampBatch(paths, 2)
	if !reflect.DeepEqual(got, []string{"a", "b"}) || !truncated {
		t.Errorf("clampBatch(paths, 2) = %v, %v; want [a b], true", got, truncated)
	}

	got, truncated = clampBatch(paths, 10)
	if !reflect.DeepEqual(got, paths) || truncated {
		t.Errorf("clampBatch(paths, 10) = %v, %v; want %v, false", got, truncated, paths)
	}

	got, truncated = clampBatch(paths, 0)
	if !reflect.DeepEqual(got, paths) || truncated {
		t.Errorf("clampBatch(paths, 0) = %v, %v; want %v, false (unlimited)", got, truncated, paths)
	}
}
