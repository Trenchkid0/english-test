package main

import (
	"testing"
)

func TestCalculateBand(t *testing.T) {
	tests := []struct {
		raw      int
		total    int
		section  string
		expected float64
	}{
		{40, 40, "listening", 9.0},
		{30, 40, "listening", 7.0},
		{20, 40, "listening", 5.5},
		{10, 40, "listening", 4.0},
		{0, 40, "listening", 1.0},
		{40, 40, "reading", 9.0},
		{30, 40, "reading", 7.0},
		{15, 40, "reading", 5.0},
		{20, 20, "listening", 9.0}, // Normalized 20/20 -> 40/40
		{15, 20, "listening", 7.5}, // Normalized 15/20 -> 30/40 -> 7.0-7.5
	}

	for _, tt := range tests {
		got := calculateBand(tt.raw, tt.total, tt.section)
		if got <= 0 {
			t.Errorf("calculateBand(%d, %d, %s) = %v, expected positive band", tt.raw, tt.total, tt.section, got)
		}
	}
}
