package gemini

import (
	"context"
	"testing"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

type fakeSink struct {
	feature, model    string
	inputTk, outputTk int
	calls             int
}

func (f *fakeSink) LogGemini(feature, model string, inputTk, outputTk int) {
	f.calls++
	f.feature, f.model, f.inputTk, f.outputTk = feature, model, inputTk, outputTk
}

func TestRecord_UsesDefaultFeature(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash")
	fs := &fakeSink{}
	c.SetUsageSink(fs)
	resp := &genai.GenerateContentResponse{
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 1200, CandidatesTokenCount: 80,
		},
	}
	c.record(context.Background(), resp, "agent")
	if fs.calls != 1 || fs.feature != "agent" || fs.model != "gemini-2.5-flash" || fs.inputTk != 1200 || fs.outputTk != 80 {
		t.Fatalf("unexpected sink state: %+v", fs)
	}
}

func TestRecord_ContextOverridesFeature(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash")
	fs := &fakeSink{}
	c.SetUsageSink(fs)
	resp := &genai.GenerateContentResponse{
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 1},
	}
	ctx := usage.WithFeature(context.Background(), "knowledge")
	c.record(ctx, resp, "text")
	if fs.feature != "knowledge" {
		t.Fatalf("feature = %q, want knowledge (ctx override)", fs.feature)
	}
}

func TestRecord_NilSafe(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash") // sink 미설정
	c.record(context.Background(), nil, "agent")                             // resp nil
	c.record(context.Background(), &genai.GenerateContentResponse{}, "agent") // UsageMetadata nil
	c.SetUsageSink(&fakeSink{})
	c.record(context.Background(), &genai.GenerateContentResponse{}, "agent") // metadata nil + sink 있음 → 호출 안 함
}
