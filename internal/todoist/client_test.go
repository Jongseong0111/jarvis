package todoist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListTasks(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header=%q", got)
		}
		// v1: 필터는 /tasks/filter?query= 별도 엔드포인트
		if r.URL.Path != "/tasks/filter" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "today" {
			t.Errorf("query=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// v1: {"results":[...],"next_cursor":...} 래퍼
		_, _ = w.Write([]byte(`{"results":[{"id":"1","content":"Clone Graph","due":{"string":"오늘","date":"2026-06-19"},"project_id":"p1"}],"next_cursor":null}`))
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

func TestListTasks_noFilterUsesPlainPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// filter 가 없으면 전체 목록 /tasks (필터 엔드포인트 아님)
		if r.URL.Path != "/tasks" {
			t.Errorf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"9","content":"전체"}],"next_cursor":null}`))
	}))
	defer srv.Close()

	c := NewWithBase("tok", srv.URL)
	tasks, err := c.ListTasks(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "9" {
		t.Fatalf("got %+v", tasks)
	}
}

func TestAddTask(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header=%q", got)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type=%q", ct)
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
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header=%q", got)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
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
	} else if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("에러에 응답 본문이 없음: %v", err)
	}
}
