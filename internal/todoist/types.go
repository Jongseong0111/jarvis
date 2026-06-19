// Package todoist 는 Todoist REST API thin 클라이언트다.
package todoist

// Task 는 Todoist 할일 1건(표시에 필요한 필드만).
type Task struct {
	ID      string
	Content string
	Due     string // 표시용 마감 문자열(없으면 "")
	Project string // 프로젝트 ID(있으면)
	URL     string // 앱에서 열 링크
}
