package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

type fakeFetcher struct {
	items []devdigest.NewsItem
	err   error
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]devdigest.NewsItem, error) {
	return f.items, f.err
}

type fakeGenerator struct {
	result devdigest.DigestResult
	err    error
}

func (g *fakeGenerator) Generate(_ context.Context, _ []devdigest.NewsItem) (devdigest.DigestResult, error) {
	return g.result, g.err
}

func TestNewDevDigestBriefing_sendsFormattedMessage(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{items: []devdigest.NewsItem{{Title: "기사", URL: "https://ex.com", Desc: "설명"}}}
	generator := &fakeGenerator{result: devdigest.DigestResult{
		News:   []devdigest.NewsResult{{Title: "Go 1.25", URL: "https://go.dev", Summary: "새 기능"}},
		Domain: "언어",
		Topics: []string{"언어 → Go → goroutine 스케줄러"},
	}}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C1")(context.Background())

	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", sender.sent)
	}
	text := sender.sent[0].Text
	if !strings.Contains(text, "개발 소식") {
		t.Fatalf("뉴스 헤더 없음: %q", text)
	}
	if !strings.Contains(text, "Go 1.25") || !strings.Contains(text, "https://go.dev") {
		t.Fatalf("뉴스 내용 없음: %q", text)
	}
	if !strings.Contains(text, "공부 주제") {
		t.Fatalf("공부 주제 헤더 없음: %q", text)
	}
	if !strings.Contains(text, "goroutine") {
		t.Fatalf("주제 내용 없음: %q", text)
	}
	if sender.sent[0].ChannelID != "C1" {
		t.Fatalf("channel=%q", sender.sent[0].ChannelID)
	}
}

func TestNewDevDigestBriefing_fetchFailStillGenerates(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{err: fmt.Errorf("네트워크 오류")}
	generator := &fakeGenerator{result: devdigest.DigestResult{
		Domain: "AI",
		Topics: []string{"AI → LLM → Transformer"},
	}}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C2")(context.Background())
	// fetch 실패해도 generate 는 시도(빈 items 로)하고 전송
	if len(sender.sent) != 1 {
		t.Fatalf("fetch 실패해도 메시지 기대: %+v", sender.sent)
	}
}

func TestNewDevDigestBriefing_generateFailSilent(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{}
	generator := &fakeGenerator{err: fmt.Errorf("gemini 오류")}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C3")(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("generate 실패 시 무음 기대: %+v", sender.sent)
	}
}
