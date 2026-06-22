package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

// fakeTodoist 는 todoist_tools_test.go 에 정의돼 있다(같은 package agent).

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
	NewMorningBriefing(f, nil, s, "C1")(context.Background())
	if len(s.sent) != 1 || !strings.Contains(s.sent[0].Text, "없습니다") || !strings.Contains(s.sent[0].Text, "좋은 하루") {
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

func TestMorningBriefing_nonEmptySends(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "1", Content: "운동", Due: "오늘"}}}
	s := &capSender{}
	NewMorningBriefing(f, nil, s, "C1")(context.Background())
	if len(s.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", s.sent)
	}
	if !strings.Contains(s.sent[0].Text, "운동") || s.sent[0].ChannelID != "C1" {
		t.Fatalf("reply=%+v", s.sent[0])
	}
}

func TestEveningBriefing_nonEmptySends(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "2", Content: "리뷰"}}}
	s := &capSender{}
	NewEveningBriefing(f, s, "C1")(context.Background())
	if len(s.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", s.sent)
	}
	if !strings.Contains(s.sent[0].Text, "리뷰") {
		t.Fatalf("reply=%+v", s.sent[0])
	}
}
