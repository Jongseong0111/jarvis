package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

// sysCapturingGen 은 넘어온 system 문자열을 기록하는 fake generator 다.
type sysCapturingGen struct{ gotSystem string }

func (g *sysCapturingGen) GenerateWithTools(ctx context.Context, contents []*genai.Content, tools []*genai.Tool, system string) (*genai.GenerateContentResponse, error) {
	g.gotSystem = system
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Parts: []*genai.Part{{Text: "네"}}},
	}}}, nil
}

func TestRoute_InjectsCurrentDate(t *testing.T) {
	t.Parallel()
	gen := &sysCapturingGen{}
	ag := New(gen, nil, nil, "")
	ag.now = func() time.Time { return time.Date(2026, 6, 22, 14, 30, 0, 0, time.UTC) }
	_, err := ag.Route(context.Background(), domain.IncomingMessage{ChannelID: "c1", Text: "안녕"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !strings.Contains(gen.gotSystem, "2026-06-22") {
		t.Fatalf("system 에 현재 날짜 미주입:\n%s", gen.gotSystem)
	}
	if !strings.Contains(gen.gotSystem, DefaultSystemPrompt) {
		t.Fatal("system 에 기본 프롬프트가 보존되지 않음")
	}
}
