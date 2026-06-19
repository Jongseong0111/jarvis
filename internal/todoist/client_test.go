package todoist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTasks(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header=%q", got)
		}
		if got := r.URL.Query().Get("filter"); got != "today" {
			t.Errorf("filter=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1","content":"Clone Graph","due":{"string":"오늘"},"project_id":"p1","url":"https://todoist.com/showTask?id=1"}]`))
	}))
	defer srv.Close()

	c := NewWithBase("tok", srv.URL)
	tasks, err := c.ListTasks(context.Background(), "today")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Content != "Clone Graph" || tasks[0].Due != "오늘" || tasks[0].ID != "1" {
		t.Fatalf("got %+v", tasks)
	}
}

func TestAddTask(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["content"] != "새 할일" || body["due_string"] != "내일" {
			t.Errorf("body=%+v", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"9","content":"새 할일","due":{"string":"내일"}}`))
	}))
	defer srv.Close()

	c := NewWithBase("tok", srv.URL)
	task, err := c.AddTask(context.Background(), "새 할일", "내일", "")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "9" || task.Content != "새 할일" {
		t.Fatalf("got %+v", task)
	}
}

func TestCompleteTask(t *testing.T) {
	t.Parallel()
	var hitPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewWithBase("tok", srv.URL)
	if err := c.CompleteTask(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	// NewWithBase 는 base 를 srv.URL 로 쓰므로 r.URL.Path 는 "/tasks/1/close" 다.
	// (defaultBase 의 "/rest/v2" 는 New() 에서만 붙는다.)
	if hitPath != "/tasks/1/close" {
		t.Fatalf("path=%s", hitPath)
	}
}

func TestDeleteTask_non2xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := NewWithBase("tok", srv.URL)
	if err := c.DeleteTask(context.Background(), "1"); err == nil {
		t.Fatal("에러를 기대했지만 nil")
	}
}
