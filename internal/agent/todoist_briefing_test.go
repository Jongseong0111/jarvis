package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

func TestFormatBriefing(t *testing.T) {
	t.Parallel()
	out := formatBriefing("☀️ 오늘 할일", []todoist.Task{
		{Content: "A", Due: "오늘"},
		{Content: "B"},
	})
	if !strings.Contains(out, "☀️ 오늘 할일") || !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Fatalf("out=%q", out)
	}
}

type capSender struct{ sent []domain.Reply }

func (c *capSender) Send(_ context.Context, r domain.Reply) error {
	c.sent = append(c.sent, r)
	return nil
}

func TestMorningBriefing_emptySendsNothingNote(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: nil}
	s := &capSender{}
	NewMorningBriefing(f, s, "C1")(context.Background())
	if len(s.sent) != 1 || !strings.Contains(s.sent[0].Text, "할일 없") {
		t.Fatalf("아침은 빈 날도 안내 전송: %+v", s.sent)
	}
}

func TestEveningBriefing_emptySilent(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: nil}
	s := &capSender{}
	NewEveningBriefing(f, s, "C1")(context.Background())
	if len(s.sent) != 0 {
		t.Fatalf("저녁은 빈 날 무음이어야 함: %+v", s.sent)
	}
}
