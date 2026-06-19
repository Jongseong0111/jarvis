package scheduler

import (
	"testing"
	"time"
)

func TestNextFire(t *testing.T) {
	t.Parallel()
	seoul, _ := time.LoadLocation("Asia/Seoul")
	tests := []struct {
		name string
		now  time.Time
		h, m int
		want time.Time
	}{
		{
			name: "오늘 아직 안 지남 → 오늘",
			now:  time.Date(2026, 6, 18, 7, 0, 0, 0, seoul),
			h:    8, m: 0,
			want: time.Date(2026, 6, 18, 8, 0, 0, 0, seoul),
		},
		{
			name: "오늘 이미 지남 → 내일",
			now:  time.Date(2026, 6, 18, 9, 0, 0, 0, seoul),
			h:    8, m: 0,
			want: time.Date(2026, 6, 19, 8, 0, 0, 0, seoul),
		},
		{
			name: "정각 동일 → 내일(경계)",
			now:  time.Date(2026, 6, 18, 8, 0, 0, 0, seoul),
			h:    8, m: 0,
			want: time.Date(2026, 6, 19, 8, 0, 0, 0, seoul),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nextFire(tt.now, tt.h, tt.m, seoul)
			if !got.Equal(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
