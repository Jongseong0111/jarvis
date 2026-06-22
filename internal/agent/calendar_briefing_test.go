package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/internal/gcal"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

// errTodoist 는 ListTasks 가 항상 오류를 반환하는 테스트용 fake.
// fakeTodoist 는 오류를 반환할 수 없으므로 별도 정의.
type errTodoist struct{}

func (e *errTodoist) ListTasks(_ context.Context, _ string) ([]todoist.Task, error) {
	return nil, errors.New("todoist API 오류(테스트)")
}
func (e *errTodoist) AddTask(_ context.Context, _, _, _ string) (todoist.Task, error) {
	return todoist.Task{}, nil
}
func (e *errTodoist) CompleteTask(_ context.Context, _ string) error { return nil }
func (e *errTodoist) UpdateTask(_ context.Context, _, _, _ string) error { return nil }
func (e *errTodoist) DeleteTask(_ context.Context, _ string) error { return nil }

// capSender 는 todoist_briefing_test.go 에 정의돼 있다(같은 package agent).
// fakeTodoist 는 todoist_tools_test.go 에 정의돼 있다(같은 package agent).
// fakeCalPort 는 calendar_tools_test.go 에 정의돼 있다(같은 package agent).

func TestMorningBriefing_WithCalendar(t *testing.T) {
	t.Parallel()
	// 할일 없음 + 일정 있음 → 일정 섹션만 포함
	todo := &fakeTodoist{tasks: nil}
	cal := &fakeCalPort{events: []gcal.Event{{Summary: "스탠드업", Start: time.Now()}}}
	sender := &capSender{}
	job := NewMorningBriefing(todo, cal, sender, "C1")
	job(context.Background())
	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대, got %d: %+v", len(sender.sent), sender.sent)
	}
	if !strings.Contains(sender.sent[0].Text, "스탠드업") {
		t.Fatalf("브리핑에 일정 없음:\n%s", sender.sent[0].Text)
	}
}

func TestMorningBriefing_CalendarAndTodos(t *testing.T) {
	t.Parallel()
	// 할일+일정 둘 다 있음 → 둘 다 포함
	todo := &fakeTodoist{tasks: []todoist.Task{{Content: "운동"}}}
	cal := &fakeCalPort{events: []gcal.Event{{Summary: "스탠드업", Start: time.Now()}}}
	sender := &capSender{}
	job := NewMorningBriefing(todo, cal, sender, "C1")
	job(context.Background())
	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", sender.sent)
	}
	if !strings.Contains(sender.sent[0].Text, "스탠드업") || !strings.Contains(sender.sent[0].Text, "운동") {
		t.Fatalf("브리핑에 일정+할일 둘 다 없음:\n%s", sender.sent[0].Text)
	}
}

func TestMorningBriefing_NilCalendar(t *testing.T) {
	t.Parallel()
	// nil cal → 기존 동작: 할일 없으면 "좋은 하루" 메시지
	todo := &fakeTodoist{tasks: nil}
	sender := &capSender{}
	job := NewMorningBriefing(todo, nil, sender, "C1")
	job(context.Background())
	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", sender.sent)
	}
	if !strings.Contains(sender.sent[0].Text, "좋은 하루") {
		t.Fatalf("nil cal 시 기존 동작(좋은 하루) 누락:\n%s", sender.sent[0].Text)
	}
}

func TestMorningBriefing_NilCalendarWithTodos(t *testing.T) {
	t.Parallel()
	// nil cal + 할일 있음 → 할일 섹션만(기존 동작 그대로)
	todo := &fakeTodoist{tasks: []todoist.Task{{Content: "운동"}}}
	sender := &capSender{}
	job := NewMorningBriefing(todo, nil, sender, "C1")
	job(context.Background())
	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", sender.sent)
	}
	if !strings.Contains(sender.sent[0].Text, "운동") {
		t.Fatalf("할일 브리핑 누락:\n%s", sender.sent[0].Text)
	}
}

func TestMorningBriefing_ListTasksError_NoCalendar_Silent(t *testing.T) {
	t.Parallel()
	// 회귀 테스트: ListTasks 오류 + 일정 없음 → 무음(메시지 전송 없음)
	// 이전 버전은 오류 시 "오늘 마감할 일이 없습니다" 를 전송해 오해를 줬다.
	todo := &errTodoist{}
	sender := &capSender{}
	job := NewMorningBriefing(todo, nil, sender, "C1")
	job(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("오류 시 무음 기대, 그런데 메시지 전송됨: %+v", sender.sent)
	}
}
