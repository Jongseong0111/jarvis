package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/todoist"
)

type fakeTodoist struct {
	tasks          []todoist.Task
	added          *todoist.Task
	completed      string
	deletedIDs     []string
	updatedID      string
	updatedContent string
	updatedDue     string
}

func (f *fakeTodoist) ListTasks(_ context.Context, _ string) ([]todoist.Task, error) {
	return f.tasks, nil
}
func (f *fakeTodoist) AddTask(_ context.Context, content, due, _ string) (todoist.Task, error) {
	t := todoist.Task{ID: "new", Content: content, Due: due}
	f.added = &t
	return t, nil
}
func (f *fakeTodoist) CompleteTask(_ context.Context, id string) error { f.completed = id; return nil }
func (f *fakeTodoist) UpdateTask(_ context.Context, id, content, due string) error {
	f.updatedID, f.updatedContent, f.updatedDue = id, content, due
	return nil
}
func (f *fakeTodoist) DeleteTask(_ context.Context, id string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func toolByName(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Decl.Name == name {
			return t
		}
	}
	return Tool{}
}

func TestAddTodoTool(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{}
	tool := toolByName(TodoistTools(f), "add_todo")
	out, err := tool.Run(context.Background(), map[string]any{"content": "Clone Graph", "due": "오늘"})
	if err != nil {
		t.Fatal(err)
	}
	if f.added == nil || f.added.Content != "Clone Graph" {
		t.Fatalf("added=%+v", f.added)
	}
	if !strings.Contains(out, "Clone Graph") {
		t.Fatalf("out=%q", out)
	}
}

func TestCompleteTodoTool_resolvesByQuery(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "7", Content: "Clone Graph 다시 풀기"}}}
	tool := toolByName(TodoistTools(f), "complete_todo")
	if _, err := tool.Run(context.Background(), map[string]any{"query": "clone graph"}); err != nil {
		t.Fatal(err)
	}
	if f.completed != "7" {
		t.Fatalf("completed=%q", f.completed)
	}
}

func TestCompleteTodoTool_ambiguous(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "1", Content: "운동 가기"}, {ID: "2", Content: "운동 기록"}}}
	tool := toolByName(TodoistTools(f), "complete_todo")
	_, err := tool.Run(context.Background(), map[string]any{"query": "운동"})
	if err == nil {
		t.Fatal("모호하면 에러(되묻기)를 기대")
	}
}

func TestListTodosTool(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "1", Content: "A", Due: "오늘"}}}
	tool := toolByName(TodoistTools(f), "list_todos")
	out, err := tool.Run(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "A") {
		t.Fatalf("out=%q", out)
	}
}

func TestDeleteTodoTool_proposes(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "5", Content: "지울 할일"}}}
	tool := toolByName(TodoistTools(f), "delete_todo")
	if !tool.Write {
		t.Fatal("delete_todo 는 Write 여야 함")
	}
	p, err := tool.Propose(context.Background(), map[string]any{"query": "지울"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Op != "delete_todo" || p.Fields["task_id"] != "5" {
		t.Fatalf("proposal=%+v", p)
	}
}

func TestListTodosTool_empty(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: nil}
	tool := toolByName(TodoistTools(f), "list_todos")
	out, err := tool.Run(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "할일이 없어." {
		t.Fatalf("out=%q, want '할일이 없어.'", out)
	}
}

func TestUpdateTodoTool(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "3", Content: "옛 이름"}}}
	tool := toolByName(TodoistTools(f), "update_todo")
	out, err := tool.Run(context.Background(), map[string]any{"query": "옛", "content": "새 이름"})
	if err != nil {
		t.Fatal(err)
	}
	if f.updatedID != "3" || f.updatedContent != "새 이름" {
		t.Fatalf("port got id=%q content=%q", f.updatedID, f.updatedContent)
	}
	if !strings.Contains(out, "새 이름") {
		t.Fatalf("메시지가 새 내용을 안 보여줌: %q", out)
	}
}
