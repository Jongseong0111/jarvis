package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

type fakeUsageReader struct{ s usage.Summary }

func (f fakeUsageReader) Query(from, to time.Time) (usage.Summary, error) { return f.s, nil }

func TestListUsageTool(t *testing.T) {
	t.Parallel()
	s := usage.Summary{
		TotalCost: 0.0123, TotalCalls: 47,
		ByModel: []usage.Bucket{
			{Key: "gemini-2.5-flash", Calls: 31, CostUSD: 0.0098},
			{Key: "claude", Calls: 4, CostUSD: 0.0014},
		},
		ByFeature: []usage.Bucket{
			{Key: "agent", Calls: 31, CostUSD: 0.0090},
			{Key: "kb", Calls: 4, CostUSD: 0.0014},
		},
	}
	tools := UsageTools(fakeUsageReader{s: s})
	if len(tools) != 1 || tools[0].Decl.Name != "list_usage" {
		t.Fatalf("want 1 list_usage tool, got %+v", tools)
	}
	if tools[0].Write {
		t.Fatal("list_usage must be read-only")
	}
	out, err := tools[0].Run(context.Background(), map[string]any{"period": "today"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"오늘", "$0.0123", "47", "gemini-2.5-flash", "agent", "•"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
