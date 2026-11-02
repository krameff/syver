//go:build !windows

package system

import "testing"

func TestUsagePercent(t *testing.T) {
	tests := []struct {
		name        string
		free, total uint64
		want        int
	}{
		{"zero blocks is zero usage", 0, 0, 0},
		{"empty", 100, 100, 0},
		{"full", 0, 100, 100},
		{"half", 50, 100, 50},
		{"rounds", 333, 1000, 67},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usagePercent(tt.free, tt.total); got != tt.want {
				t.Errorf("usagePercent(%d, %d) = %d, want %d", tt.free, tt.total, got, tt.want)
			}
		})
	}
}
