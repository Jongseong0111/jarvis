package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/notion"
)

// --- fakes ---

type fakeGen struct {
	responses []*genai.GenerateContentResponse
	i         int
	calls     int
}

func (f *fakeGen) GenerateWithTools(_ context.Context, _ []*genai.Content, _ []*genai.Tool, _ string) (*genai.GenerateContentResponse, error) {
	f.calls++
	r := f.responses[f.i%len(f.responses)]
	f.i++
	return r, nil
}

func textResp(s string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: s}}},
	}}}
}

func callResp(name string, args map[string]any) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Role: "model", Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: name, Args: args}}}},
	}}}
}

type fakeHomePort struct {
	locations  []notion.Location
	categories []notion.Category
	items      []notion.Item
	search     []notion.Item
	createdLoc *[2]string // name, zone
}

func (f *fakeHomePort) Locations(context.Context) ([]notion.Location, error) { return f.locations, nil }
func (f *fakeHomePort) Categories(context.Context) ([]notion.Category, error) {
	return f.categories, nil
}
func (f *fakeHomePort) Items(context.Context) ([]notion.Item, error) { return f.items, nil }
func (f *fakeHomePort) SearchItems(context.Context, string) ([]notion.Item, error) {
	return f.search, nil
}
func (f *fakeHomePort) CreateItem(_ context.Context, _, _, _, _ string, _ *int) (string, error) {
	return "item-id", nil
}
func (f *fakeHomePort) CreateLocation(_ context.Context, name, zone string) (string, error) {
	f.createdLoc = &[2]string{name, zone}
	return "loc-id", nil
}

func newAgent(gen generator, port HomePort) Agent {
	return New(gen, HomeTools(port), "")
}

// --- tests ---

func TestAgent_chat(t *testing.T) {
	t.Parallel()
	a := newAgent(&fakeGen{responses: []*genai.GenerateContentResponse{textResp("안녕! 뭐 도와줄까?")}}, &fakeHomePort{})
	reply, err := a.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "안녕"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if reply.Text != "안녕! 뭐 도와줄까?" || len(reply.Buttons) != 0 {
		t.Fatalf("잡담 응답 = %+v", reply)
	}
}

func TestAgent_readTool(t *testing.T) {
	t.Parallel()
	port := &fakeHomePort{
		locations: []notion.Location{{ID: "loc-1", Name: "아기 트롤리", Zone: "거실"}},
		search:    []notion.Item{{Name: "체온계", LocationID: "loc-1", Zone: "거실"}},
	}
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{
		callResp("search_item", map[string]any{"name": "체온계"}),
		textResp("체온계는 거실(아기 트롤리)에 있어."),
	}}
	a := newAgent(gen, port)
	reply, err := a.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "체온계 어디?"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if gen.calls != 2 {
		t.Fatalf("gen 호출 수 = %d, want 2 (도구실행 후 재호출)", gen.calls)
	}
	if !strings.Contains(reply.Text, "거실") {
		t.Fatalf("응답 = %q", reply.Text)
	}
}

func TestAgent_writeTool_proposal(t *testing.T) {
	t.Parallel()
	port := &fakeHomePort{} // 팬트리 없음
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{
		callResp("add_location", map[string]any{"name": "팬트리", "zone": "거실복도"}),
	}}
	a := newAgent(gen, port)
	reply, err := a.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "팬트리를 거실복도에 추가"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(reply.Buttons) != 2 {
		t.Fatalf("변경안 버튼이 없음: %+v", reply)
	}
	p, err := domain.DecodeProposal(reply.Buttons[0].Value)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Op != "add_location" || p.Fields["name"] != "팬트리" || p.Fields["zone"] != "거실복도" {
		t.Fatalf("변경안 = %+v", p)
	}
}

func TestAgent_writeResolveFail_asksBack(t *testing.T) {
	t.Parallel()
	port := &fakeHomePort{locations: []notion.Location{{ID: "loc-1", Name: "아기 트롤리", Zone: "거실"}}}
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{
		callResp("add_item", map[string]any{"name": "체온계", "location": "없는장소"}),
		textResp("'없는장소'는 등록돼 있지 않아. 새로 만들까?"),
	}}
	a := newAgent(gen, port)
	reply, err := a.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "없는장소에 체온계"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(reply.Buttons) != 0 {
		t.Fatal("resolve 실패 시 버튼이 없어야 함(되묻기)")
	}
	if !strings.Contains(reply.Text, "없는장소") {
		t.Fatalf("되묻기 응답 = %q", reply.Text)
	}
}

func TestAgent_maxTurns(t *testing.T) {
	t.Parallel()
	// 항상 읽기 도구만 호출 → 루프 상한 도달
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{
		callResp("list_zones", map[string]any{}),
	}}
	a := newAgent(gen, &fakeHomePort{})
	reply, err := a.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "루프"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if gen.calls != maxTurns {
		t.Fatalf("gen 호출 수 = %d, want %d", gen.calls, maxTurns)
	}
	if !strings.Contains(reply.Text, "복잡한") {
		t.Fatalf("상한 응답 = %q", reply.Text)
	}
}

func TestHomeApplier_addLocation(t *testing.T) {
	t.Parallel()
	port := &fakeHomePort{}
	reply, err := NewHomeApplier(port).Apply(context.Background(), domain.ChangeProposal{
		Op: "add_location", Fields: map[string]string{"name": "팬트리", "zone": "거실복도"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if port.createdLoc == nil || port.createdLoc[0] != "팬트리" || port.createdLoc[1] != "거실복도" {
		t.Fatalf("CreateLocation 인자 = %v", port.createdLoc)
	}
	if !strings.Contains(reply.Text, "추가했어") {
		t.Fatalf("확인 응답 = %q", reply.Text)
	}
}
