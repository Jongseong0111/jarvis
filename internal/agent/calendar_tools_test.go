package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/internal/gcal"
)

type fakeCalPort struct {
	events    []gcal.Event
	added     gcal.Event
	deletedID string
	lastMin   time.Time
	lastMax   time.Time
	lastQuery string
}

func (f *fakeCalPort) ListEvents(ctx context.Context, mn, mx time.Time) ([]gcal.Event, error) {
	f.lastMin, f.lastMax = mn, mx
	return f.events, nil
}
func (f *fakeCalPort) SearchEvents(ctx context.Context, q string, mn, mx time.Time) ([]gcal.Event, error) {
	f.lastQuery = q
	return f.events, nil
}
func (f *fakeCalPort) AddEvent(ctx context.Context, ev gcal.Event) (gcal.Event, error) {
	f.added = ev
	ev.ID = "new1"
	return ev, nil
}
func (f *fakeCalPort) DeleteEvent(ctx context.Context, id string) error {
	f.deletedID = id
	return nil
}

func TestListEventsTool_Format(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{events: []gcal.Event{
		{Summary: "미팅", Start: time.Date(2026, 6, 23, 15, 0, 0, 0, time.UTC)},
		{Summary: "검진", AllDay: true, Start: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)},
	}}
	tool := toolByName(CalendarTools(port), "list_events")
	out, err := tool.Run(context.Background(), map[string]any{"period": "week"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"미팅", "검진", "•"} {
		if !strings.Contains(out, want) {
			t.Fatalf("출력에 %q 없음:\n%s", want, out)
		}
	}
}

func TestAddEventTool_TimedAndAllDay(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{}
	tool := toolByName(CalendarTools(port), "add_event")

	// 타임드: end 없으면 +1h
	if _, err := tool.Run(context.Background(), map[string]any{"summary": "회의", "start": "2026-06-29T15:00:00+09:00"}); err != nil {
		t.Fatalf("Run timed: %v", err)
	}
	if port.added.AllDay || port.added.Summary != "회의" {
		t.Fatalf("타임드 add 오류: %+v", port.added)
	}
	if got := port.added.End.Sub(port.added.Start); got != time.Hour {
		t.Fatalf("기본 종료가 +1h 아님: %v", got)
	}

	// 종일: 날짜만
	if _, err := tool.Run(context.Background(), map[string]any{"summary": "검진", "start": "2026-07-03"}); err != nil {
		t.Fatalf("Run all-day: %v", err)
	}
	if !port.added.AllDay {
		t.Fatalf("종일 플래그 미설정: %+v", port.added)
	}
	// 종일 end는 start + 1일
	if port.added.End != port.added.Start.AddDate(0, 0, 1) {
		t.Fatalf("종일 end가 +1일 아님: start=%v, end=%v", port.added.Start, port.added.End)
	}
}

func TestDeleteEventTool_Proposes(t *testing.T) {
	t.Parallel()
	// 고정된 시간을 사용하여 테스트 결정성 보장
	fixedTime := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	port := &fakeCalPort{events: []gcal.Event{{ID: "e9", Summary: "치과 예약", Start: fixedTime}}}
	tool := toolByName(CalendarTools(port), "delete_event")
	if !tool.Write {
		t.Fatal("delete_event 는 Write=true 여야 함")
	}
	p, err := tool.Propose(context.Background(), map[string]any{"query": "치과"})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if p.Op != "delete_event" || p.Fields["event_id"] != "e9" {
		t.Fatalf("변경안 오류: %+v", p)
	}
	// 제안의 summary 필드 검증
	if p.Fields["summary"] != "치과 예약" {
		t.Fatalf("summary 불일치: got %q, want \"치과 예약\"", p.Fields["summary"])
	}
}

func TestEventRange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC)
	min, max := eventRange(now, "today")
	if min.Day() != 22 || max.Sub(min) != 24*time.Hour {
		t.Fatalf("today 범위 오류: %v ~ %v", min, max)
	}
	_, wmax := eventRange(now, "week")
	if wmax.Sub(now) < 6*24*time.Hour {
		t.Fatalf("week 범위가 너무 짧음: %v", wmax.Sub(now))
	}

	// tomorrow: 2026-06-23 00:00:00부터 2026-06-24 00:00:00까지 (24h)
	tmin, tmax := eventRange(now, "tomorrow")
	expectedTmin := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	if tmin != expectedTmin {
		t.Fatalf("tomorrow min 오류: got %v, want %v", tmin, expectedTmin)
	}
	if tmax.Sub(tmin) != 24*time.Hour {
		t.Fatalf("tomorrow 범위가 24h 아님: %v", tmax.Sub(tmin))
	}

	// month: 30일 윈도우 (30 * 24h)
	mmin, mmax := eventRange(now, "month")
	if mmax.Sub(mmin) != 30*24*time.Hour {
		t.Fatalf("month 범위가 30일 아님: %v", mmax.Sub(mmin))
	}
}
