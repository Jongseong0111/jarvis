package usage

import (
	"math"
	"testing"
)

func TestGeminiCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		model      string
		inTk, outTk int
		want       float64
	}{
		{"flash", "gemini-2.5-flash", 1_000_000, 1_000_000, 0.30 + 2.50},
		{"flash-lite", "gemini-2.5-flash-lite", 1_000_000, 1_000_000, 0.10 + 0.40},
		{"flash partial", "gemini-2.5-flash", 2000, 100, 0.30*2000/1e6 + 2.50*100/1e6},
		{"unknown model -> 0", "gemini-9.9-ultra", 5000, 5000, 0},
		{"zero tokens", "gemini-2.5-flash", 0, 0, 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := geminiCost(tt.model, tt.inTk, tt.outTk)
			if math.Abs(got-tt.want) > 1e-12 {
				t.Fatalf("geminiCost(%q,%d,%d) = %v, want %v", tt.model, tt.inTk, tt.outTk, got, tt.want)
			}
		})
	}
}
