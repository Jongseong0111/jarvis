package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

func TestTodoistApplier_delete(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: []todoist.Task{{ID: "5", Content: "x"}}}
	ap := NewTodoistApplier(f)
	reply, err := ap.Apply(context.Background(), domain.ChangeProposal{
		Op:     "delete_todo",
		Fields: map[string]string{"task_id": "5", "content": "지울 할일"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.deletedIDs) != 1 || f.deletedIDs[0] != "5" {
		t.Fatalf("deleted=%v", f.deletedIDs)
	}
	if !strings.Contains(reply.Text, "지울 할일") {
		t.Fatalf("reply=%q", reply.Text)
	}
}

type stubApplier struct{ called bool }

func (s *stubApplier) Apply(_ context.Context, _ domain.ChangeProposal) (domain.Reply, error) {
	s.called = true
	return domain.Reply{Text: "fallback"}, nil
}

func TestDispatchApplier_routesByOp(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{}
	fallback := &stubApplier{}
	ap := NewDispatchApplier(map[string]domain.ProposalApplier{
		"delete_todo": NewTodoistApplier(f),
	}, fallback)

	// delete_todo → todoist
	if _, err := ap.Apply(context.Background(), domain.ChangeProposal{Op: "delete_todo", Fields: map[string]string{"task_id": "1"}}); err != nil {
		t.Fatal(err)
	}
	if fallback.called {
		t.Fatal("delete_todo 가 fallback 으로 갔음")
	}
	// 그 외 → fallback
	if _, err := ap.Apply(context.Background(), domain.ChangeProposal{Op: "add_item"}); err != nil {
		t.Fatal(err)
	}
	if !fallback.called {
		t.Fatal("add_item 이 fallback 으로 안 갔음")
	}
}
