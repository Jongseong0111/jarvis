// Package gcal 은 Google Calendar(공식 SDK)를 감싸는 어댑터다.
package gcal

import "time"

// Event 는 캘린더 일정의 도메인 표현이다.
type Event struct {
	ID       string
	Summary  string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Location string
}
