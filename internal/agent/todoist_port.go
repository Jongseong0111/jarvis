package agent

import (
	"context"

	"github.com/Jongseong0111/jarvis/internal/todoist"
)

// TodoistPort 는 Todoist 도구가 필요로 하는 작업이다(테스트 fake 주입).
type TodoistPort interface {
	ListTasks(ctx context.Context, filter string) ([]todoist.Task, error)
	AddTask(ctx context.Context, content, due, project string) (todoist.Task, error)
	CompleteTask(ctx context.Context, id string) error
	UpdateTask(ctx context.Context, id, content, due string) error
	DeleteTask(ctx context.Context, id string) error
}
