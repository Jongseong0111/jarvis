package devdigest

import (
	"context"
	"strings"
	"testing"
)

func TestParseResponse_valid(t *testing.T) {
	t.Parallel()
	raw := `{"news":[{"title":"Go 1.25","url":"https://go.dev","summary":"새 기능 출시"}],"domain":"언어","topics":["언어 → Go → goroutine 스케줄러"]}`
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "언어" {
		t.Fatalf("domain=%q", result.Domain)
	}
	if len(result.News) != 1 || result.News[0].URL != "https://go.dev" {
		t.Fatalf("news=%+v", result.News)
	}
	if len(result.Topics) != 1 || !strings.Contains(result.Topics[0], "goroutine") {
		t.Fatalf("topics=%+v", result.Topics)
	}
}

func TestParseResponse_markdownWrapped(t *testing.T) {
	t.Parallel()
	raw := "```json\n{\"news\":[],\"domain\":\"AI\",\"topics\":[\"AI → LLM → Transformer 구조\"]}\n```"
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "AI" {
		t.Fatalf("domain=%q", result.Domain)
	}
}

func TestParseResponse_invalid(t *testing.T) {
	t.Parallel()
	_, err := parseResponse("not json")
	if err == nil {
		t.Fatal("잘못된 JSON 에 error 기대")
	}
}

func TestBuildPrompt_containsItems(t *testing.T) {
	t.Parallel()
	items := []NewsItem{
		{Title: "기사A", URL: "https://a.com", Desc: "설명A"},
	}
	p := buildPrompt(items)
	if !strings.Contains(p, "기사A") || !strings.Contains(p, "https://a.com") {
		t.Fatalf("prompt=%q", p)
	}
}

// 컴파일 검증: Generator 인터페이스를 GeminiGenerator 가 구현하는지 확인한다.
var _ Generator = (*GeminiGenerator)(nil)

// 컴파일 검증: Generate 시그니처 확인.
var _ = func(g *GeminiGenerator) {
	var _ func(context.Context, []NewsItem) (DigestResult, error) = g.Generate
}
