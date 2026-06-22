package usage

import (
	"testing"
	"time"
)

// seed 는 고정 ts 로 레코드를 직접 기록한다(now 고정).
func seedAt(r *Recorder, ts time.Time, log func()) {
	r.now = func() time.Time { return ts }
	log()
}

func TestQuery_FilterAndAggregate(t *testing.T) {
	t.Parallel()
	r, _ := fixedRecorder(t)
	base := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	seedAt(r, base, func() { r.LogGemini("agent", "gemini-2.5-flash", 1000, 100) })          // 0.0003 + 0.00025 = 0.00055
	seedAt(r, base.Add(time.Hour), func() { r.LogGemini("vision", "gemini-2.5-flash-lite", 500, 50) }) // 0.00005 + 0.00002 = 0.00007
	seedAt(r, base.Add(2*time.Hour), func() { r.LogClaude("kb", "claude", 100, 10, 0.12) })
	seedAt(r, base.AddDate(0, 0, -1), func() { r.LogGemini("agent", "gemini-2.5-flash", 999, 999) }) // 어제 → 제외

	from := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	s, err := r.Query(from, to)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.TotalCalls != 3 {
		t.Fatalf("TotalCalls = %d, want 3", s.TotalCalls)
	}
	wantCost := 0.00055 + 0.00007 + 0.12
	if d := s.TotalCost - wantCost; d < -1e-9 || d > 1e-9 {
		t.Fatalf("TotalCost = %v, want %v", s.TotalCost, wantCost)
	}
	if len(s.ByFeature) != 3 {
		t.Fatalf("ByFeature buckets = %d, want 3", len(s.ByFeature))
	}
	// 정렬: claude(kb, 0.12) 가 최상위
	if s.ByFeature[0].Key != "kb" {
		t.Fatalf("top feature = %q, want kb", s.ByFeature[0].Key)
	}
}

func TestQuery_MissingFileIsEmpty(t *testing.T) {
	t.Parallel()
	r, _ := fixedRecorder(t) // 아직 아무것도 기록 안 함 → 파일 없음
	s, err := r.Query(time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("missing file should be empty, got err %v", err)
	}
	if s.TotalCalls != 0 {
		t.Fatalf("want 0 calls, got %d", s.TotalCalls)
	}
}

func TestRangeForPeriod(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 14, 30, 0, 0, time.UTC) // 월요일
	tests := []struct{ period string; wantFrom time.Time }{
		{"today", time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)},
		{"month", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"", time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)}, // 기본 today
	}
	for _, tt := range tests {
		from, to := RangeForPeriod(now, tt.period)
		if !from.Equal(tt.wantFrom) {
			t.Errorf("period %q: from = %v, want %v", tt.period, from, tt.wantFrom)
		}
		if !to.After(now) && !to.Equal(now) {
			t.Errorf("period %q: to %v should be >= now %v", tt.period, to, now)
		}
	}
}

func TestRangeForPeriod_WeekStartsMonday(t *testing.T) {
	t.Parallel()
	// 2026-06-24 는 수요일 → 그 주 월요일은 2026-06-22.
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	from, to := RangeForPeriod(now, "week")
	wantFrom := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	if !from.Equal(wantFrom) {
		t.Fatalf("week from = %v, want %v (월요일)", from, wantFrom)
	}
	if !to.Equal(now) {
		t.Fatalf("week to = %v, want now %v", to, now)
	}
}
