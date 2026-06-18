# Todoist 양방향 연동 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slack에서 자연어로 Todoist 할일을 추가/조회/완료/수정/삭제하고, 아침·저녁 스케줄 브리핑을 Slack으로 받는다.

**Architecture:** 기존 jarvis 패턴(thin REST 클라이언트 + agent Tool + Propose 승인)을 그대로 따른다. Todoist thin 클라이언트(`internal/todoist`)를 만들고, 에이전트 도구 5종을 붙인다(추가/조회/완료/수정은 즉시 실행, 삭제는 승인 버튼). inbound는 webhook 대신 인프로세스 스케줄러(`internal/scheduler`)가 주기적으로 Todoist를 읽어 브리핑을 전송한다.

**Tech Stack:** Go, `net/http`(thin client), `google.golang.org/genai`(도구 선언), Slack Socket Mode, `time`(스케줄러). 외부 의존성 추가 없음.

## Global Constraints

- 설계 문서: `docs/superpowers/specs/2026-06-18-jarvis-todoist-integration-design.md` (이 계획의 근거).
- 코딩 컨벤션: Clean Architecture(Domain→Worker→Channel), 생성자 주입, **value receiver**, table-driven 테스트 + `t.Parallel()` + 정적 시간 주입, **한국어 주석/커밋**.
- 에러: stdlib `errors` + `fmt.Errorf("...: %w", err)`. 로깅: `log` 패키지(`log.FromContext(ctx)`).
- Go 모듈 경로: `github.com/Jongseong0111/jarvis`.
- 네트워크 호출은 `httptest`로 단위 테스트(요청 조립·응답 파싱), 실제 Todoist 호출은 라이브 수동 검증.
- `TODOIST_API_TOKEN`이 비면 Todoist 기능 전체 off — 기존 동작 회귀 0.
- 각 태스크 끝: `go test ./... && go vet ./... && gofmt -l .`(빈 출력) green 후 커밋.

---

### Task 1: Config — Todoist 설정 5종

**Files:**
- Modify: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `Config` 필드 `TodoistAPIToken, TodoistBriefingChannel string`, `TodoistMorning, TodoistEvening string`(`"HH:MM"`), `TodoistTZ string`. 파싱 헬퍼 `ParseHHMM(s string) (hour, min int, err error)`.

- [ ] **Step 1: 실패 테스트 작성** — `pkg/config/config_test.go`에 추가

```go
func TestParseHHMM(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       string
		wantH    int
		wantM    int
		wantErr  bool
	}{
		{"정상", "08:30", 8, 30, false},
		{"자정", "00:00", 0, 0, false},
		{"23시59분", "23:59", 23, 59, false},
		{"시간초과", "24:00", 0, 0, true},
		{"분초과", "08:60", 0, 0, true},
		{"형식오류", "8시", 0, 0, true},
		{"빈값", "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, err := ParseHHMM(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && (h != tt.wantH || m != tt.wantM) {
				t.Fatalf("got %d:%d, want %d:%d", h, m, tt.wantH, tt.wantM)
			}
		})
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/ -run TestParseHHMM`
Expected: FAIL — `undefined: ParseHHMM`

- [ ] **Step 3: 구현** — `pkg/config/config.go`

`Config` 구조체에 필드 추가(Notion 필드 아래):

```go
	TodoistAPIToken        string
	TodoistBriefingChannel string
	TodoistMorning         string // "HH:MM"
	TodoistEvening         string // "HH:MM"
	TodoistTZ              string
```

`New()`의 `cfg := Config{...}` 안에 로드 추가:

```go
		TodoistAPIToken:        os.Getenv("TODOIST_API_TOKEN"),
		TodoistBriefingChannel: os.Getenv("TODOIST_BRIEFING_CHANNEL"),
		TodoistMorning:         getenv("TODOIST_MORNING_TIME", "08:00"),
		TodoistEvening:         getenv("TODOIST_EVENING_TIME", "21:00"),
		TodoistTZ:              getenv("TODOIST_BRIEFING_TZ", "Asia/Seoul"),
```

파일 끝에 헬퍼 추가:

```go
// ParseHHMM 은 "HH:MM" 문자열을 시·분으로 파싱한다.
func ParseHHMM(s string) (hour, min int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("시각 형식이 HH:MM 이 아님: %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("시(hour) 가 잘못됨: %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("분(min) 이 잘못됨: %q", s)
	}
	return h, m, nil
}
```

`import` 블록에 `"strconv"` 추가(이미 `"strings"`, `"fmt"` 있음 — 확인).

- [ ] **Step 4: 통과 확인**

Run: `go test ./pkg/config/ -run TestParseHHMM`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add pkg/config/config.go pkg/config/config_test.go
git commit -m "feat(config): Todoist 설정(토큰·브리핑 채널·시각·타임존) 로드 + HH:MM 파서"
```

---

### Task 2: Todoist thin 클라이언트

**Files:**
- Create: `internal/todoist/types.go`
- Create: `internal/todoist/client.go`
- Test: `internal/todoist/client_test.go`

**Interfaces:**
- Produces:
  ```go
  type Task struct { ID, Content, Due, Project, URL string }
  func New(token string) *Client
  func NewWithBase(token, baseURL string) *Client      // 테스트용 base 주입
  func (c *Client) ListTasks(ctx context.Context, filter string) ([]Task, error)
  func (c *Client) AddTask(ctx context.Context, content, due, project string) (Task, error)
  func (c *Client) CompleteTask(ctx context.Context, id string) error
  func (c *Client) UpdateTask(ctx context.Context, id, content, due string) error
  func (c *Client) DeleteTask(ctx context.Context, id string) error
  ```

- [ ] **Step 0: 실 API 형태 확인(코드 작성 전 1회)**

Run(개인 토큰으로 수동):
```bash
curl -s -H "Authorization: Bearer $TODOIST_API_TOKEN" \
  "https://api.todoist.com/rest/v2/tasks?filter=today" | head -c 800
```
응답 JSON의 필드명(`id`, `content`, `due.date`, `due.string`, `project_id`, `url`)과 base URL을 확인. 통합 API(`/api/v1`)로 이전됐으면 `baseURL` 상수와 경로만 그에 맞게 조정(아래 코드의 경로 부분). 인증·바디 구조는 동일 가정.

- [ ] **Step 1: DTO 작성** — `internal/todoist/types.go`

```go
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
```

- [ ] **Step 2: 실패 테스트 작성** — `internal/todoist/client_test.go`

```go
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
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/todoist/`
Expected: FAIL — `undefined: NewWithBase` 등

- [ ] **Step 4: 클라이언트 구현** — `internal/todoist/client.go`

Step 0에서 확인한 base/경로 기준. 아래는 REST v2 기준(다르면 `defaultBase`와 경로만 조정):

```go
package todoist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBase = "https://api.todoist.com/rest/v2"

// Client 는 Todoist REST 클라이언트다.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New 는 기본 base URL 로 클라이언트를 만든다.
func New(token string) *Client { return NewWithBase(token, defaultBase) }

// NewWithBase 는 base URL 을 주입한다(테스트용).
func NewWithBase(token, baseURL string) *Client {
	return &Client{token: token, baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// apiTask 는 Todoist 응답 형태(내부 디코딩용).
type apiTask struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
	Due       *struct {
		String string `json:"string"`
		Date   string `json:"date"`
	} `json:"due"`
}

func (a apiTask) toTask() Task {
	due := ""
	if a.Due != nil {
		if a.Due.String != "" {
			due = a.Due.String
		} else {
			due = a.Due.Date
		}
	}
	return Task{ID: a.ID, Content: a.Content, Due: due, Project: a.ProjectID, URL: a.URL}
}

// do 는 요청을 보내고 2xx 가 아니면 에러를 만든다. respBody 가 non-nil 이면 JSON 디코딩.
func (c *Client) do(ctx context.Context, method, path string, reqBody, respBody any) error {
	var r io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("요청 직렬화: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return fmt.Errorf("요청 생성: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("요청 전송: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("todoist %s %s: %d %s", method, path, resp.StatusCode, string(body))
	}
	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("응답 파싱: %w", err)
		}
	}
	return nil
}

// ListTasks 는 필터(예: "today | overdue")에 맞는 할일을 조회한다.
func (c *Client) ListTasks(ctx context.Context, filter string) ([]Task, error) {
	path := "/tasks"
	if filter != "" {
		path += "?filter=" + url.QueryEscape(filter)
	}
	var raw []apiTask
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	tasks := make([]Task, len(raw))
	for i, a := range raw {
		tasks[i] = a.toTask()
	}
	return tasks, nil
}

// AddTask 는 할일을 추가한다. due/project 는 빈 문자열이면 생략.
func (c *Client) AddTask(ctx context.Context, content, due, project string) (Task, error) {
	body := map[string]any{"content": content}
	if due != "" {
		body["due_string"] = due
	}
	if project != "" {
		body["project_id"] = project
	}
	var raw apiTask
	if err := c.do(ctx, http.MethodPost, "/tasks", body, &raw); err != nil {
		return Task{}, err
	}
	return raw.toTask(), nil
}

// CompleteTask 는 할일을 완료 처리한다.
func (c *Client) CompleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/tasks/"+id+"/close", nil, nil)
}

// UpdateTask 는 내용/마감을 수정한다(빈 값은 변경 안 함).
func (c *Client) UpdateTask(ctx context.Context, id, content, due string) error {
	body := map[string]any{}
	if content != "" {
		body["content"] = content
	}
	if due != "" {
		body["due_string"] = due
	}
	return c.do(ctx, http.MethodPost, "/tasks/"+id, body, nil)
}

// DeleteTask 는 할일을 삭제한다.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/tasks/"+id, nil, nil)
}
```

`import` 블록에 `"net/url"` 추가.

> 주의: 테스트의 `CompleteTask` 경로 기대값 `/rest/v2/tasks/1/close`은 `NewWithBase`가 `srv.URL`을 base로 쓰므로 실제로는 `srv.URL + "/tasks/1/close"`. 테스트는 `r.URL.Path`만 보므로 base가 `srv.URL`이면 path는 `/tasks/1/close`다. **테스트의 기대 path를 `/tasks/1/close`로 수정**(위 Step 2 코드의 `hitPath` 비교를 `/tasks/1/close`로). defaultBase의 `/rest/v2`는 `New()`에서만 붙는다.

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/todoist/`
Expected: PASS (4개)

- [ ] **Step 6: 커밋**

```bash
git add internal/todoist/
git commit -m "feat(todoist): REST thin 클라이언트(List/Add/Complete/Update/Delete) + httptest"
```

---

### Task 3: TodoistPort + 읽기/즉시실행 도구 (list/add/complete/update)

**Files:**
- Create: `internal/agent/todoist_port.go`
- Create: `internal/agent/todoist_tools.go`
- Test: `internal/agent/todoist_tools_test.go`

**Interfaces:**
- Consumes: `todoist.Task`.
- Produces:
  ```go
  type TodoistPort interface {
      ListTasks(ctx context.Context, filter string) ([]todoist.Task, error)
      AddTask(ctx context.Context, content, due, project string) (todoist.Task, error)
      CompleteTask(ctx context.Context, id string) error
      UpdateTask(ctx context.Context, id, content, due string) error
      DeleteTask(ctx context.Context, id string) error
  }
  func TodoistTools(port TodoistPort) []Tool
  ```
  `*todoist.Client`가 `TodoistPort`를 그대로 만족.

- [ ] **Step 1: Port 인터페이스** — `internal/agent/todoist_port.go`

```go
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
```

- [ ] **Step 2: 실패 테스트** — `internal/agent/todoist_tools_test.go`

```go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/todoist"
)

type fakeTodoist struct {
	tasks      []todoist.Task
	added      *todoist.Task
	completed  string
	deletedIDs []string
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
func (f *fakeTodoist) UpdateTask(_ context.Context, _, _, _ string) error { return nil }
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
```

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/agent/ -run Todo`
Expected: FAIL — `undefined: TodoistTools`

- [ ] **Step 4: 도구 구현** — `internal/agent/todoist_tools.go`

```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

type todoistTools struct {
	port TodoistPort
}

// TodoistTools 는 할일 도구 목록을 만든다.
// add/list/complete/update 는 즉시 실행(Run), delete 만 변경안(Propose).
func TodoistTools(port TodoistPort) []Tool {
	t := todoistTools{port: port}
	return []Tool{t.listTodos(), t.addTodo(), t.completeTodo(), t.updateTodo(), t.deleteTodo()}
}

func (t todoistTools) listTodos() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_todos",
			Description: "할일을 조회한다. filter 미지정 시 오늘+밀린 할일. filter 는 Todoist 필터 문법(예: today, overdue, tomorrow, today | overdue).",
			Parameters: objSchema(map[string]*genai.Schema{
				"filter": strSchema("Todoist 필터(선택). 기본 'today | overdue'"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			filter := strArg(args, "filter")
			if filter == "" {
				filter = "today | overdue"
			}
			tasks, err := t.port.ListTasks(ctx, filter)
			if err != nil {
				return "", err
			}
			if len(tasks) == 0 {
				return "할일이 없어.", nil
			}
			return formatTaskLines(tasks), nil
		},
	}
}

func (t todoistTools) addTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "add_todo",
			Description: "할일을 추가한다. due 는 자연어 마감(예: '오늘', '내일 오후 3시', '매주 월요일')도 가능.",
			Parameters: objSchema(map[string]*genai.Schema{
				"content": strSchema("할일 내용. 예: Clone Graph 다시 풀기"),
				"due":     strSchema("마감(선택). 자연어 가능"),
				"project": strSchema("프로젝트 ID(선택)"),
			}, "content"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			content := strings.TrimSpace(strArg(args, "content"))
			if content == "" {
				return "", fmt.Errorf("할일 내용이 필요해")
			}
			task, err := t.port.AddTask(ctx, content, strArg(args, "due"), strArg(args, "project"))
			if err != nil {
				return "", err
			}
			if task.Due != "" {
				return fmt.Sprintf("추가했어: %s (%s)", task.Content, task.Due), nil
			}
			return "추가했어: " + task.Content, nil
		},
	}
}

func (t todoistTools) completeTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "complete_todo",
			Description: "할일을 완료 처리한다. query 로 내용을 찾는다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("완료할 할일 내용(부분일치). 예: Clone Graph"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return "", err
			}
			if err := t.port.CompleteTask(ctx, task.ID); err != nil {
				return "", err
			}
			return "완료했어: " + task.Content, nil
		},
	}
}

func (t todoistTools) updateTodo() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "update_todo",
			Description: "할일의 내용 또는 마감을 수정한다. query 로 대상을 찾는다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query":   strSchema("수정할 할일 내용(부분일치)"),
				"content": strSchema("새 내용(선택)"),
				"due":     strSchema("새 마감(선택, 자연어 가능)"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			content := strings.TrimSpace(strArg(args, "content"))
			due := strings.TrimSpace(strArg(args, "due"))
			if content == "" && due == "" {
				return "", fmt.Errorf("뭘 바꿀지 알려줘(내용/마감)")
			}
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return "", err
			}
			if err := t.port.UpdateTask(ctx, task.ID, content, due); err != nil {
				return "", err
			}
			return "수정했어: " + task.Content, nil
		},
	}
}

func (t todoistTools) deleteTodo() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_todo",
			Description: "할일을 삭제한다. query 로 대상을 찾고 승인 버튼을 거친다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("삭제할 할일 내용(부분일치)"),
			}, "query"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			task, err := t.resolveTask(ctx, strArg(args, "query"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			return domain.ChangeProposal{
				Op:      "delete_todo",
				Summary: "할일 삭제\n" + task.Content,
				Fields:  map[string]string{"task_id": task.ID, "content": task.Content},
			}, nil
		},
	}
}

// resolveTask 는 query 로 미완료 할일 1개를 찾는다. 0개/다수면 에러(되묻기).
func (t todoistTools) resolveTask(ctx context.Context, query string) (todoist.Task, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return todoist.Task{}, fmt.Errorf("어떤 할일인지 알려줘")
	}
	tasks, err := t.port.ListTasks(ctx, "")
	if err != nil {
		return todoist.Task{}, err
	}
	var matches []todoist.Task
	for _, tk := range tasks {
		if strings.Contains(strings.ToLower(tk.Content), strings.ToLower(query)) {
			matches = append(matches, tk)
		}
	}
	switch len(matches) {
	case 0:
		return todoist.Task{}, fmt.Errorf("'%s'에 해당하는 할일을 못 찾았어.", query)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.Content)
		}
		return todoist.Task{}, fmt.Errorf("'%s'에 해당하는 게 여러 개야: %s. 더 정확히 알려줄래?", query, strings.Join(names, ", "))
	}
}

// formatTaskLines 는 할일을 "• 내용 — 마감" 줄로 만든다.
func formatTaskLines(tasks []todoist.Task) string {
	var lines []string
	for _, tk := range tasks {
		line := "• " + tk.Content
		if tk.Due != "" {
			line += " — " + tk.Due
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/agent/ -run Todo`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/agent/todoist_port.go internal/agent/todoist_tools.go internal/agent/todoist_tools_test.go
git commit -m "feat(agent): Todoist 도구(list/add/complete/update 즉시실행 + delete 변경안)"
```

---

### Task 4: 삭제 승인 — TodoistApplier + DispatchApplier

**Files:**
- Modify: `internal/agent/applier.go`
- Test: `internal/agent/applier_test.go` (없으면 생성)

**Interfaces:**
- Consumes: `domain.ChangeProposal`, `domain.ProposalApplier`, `TodoistPort`(DeleteTask).
- Produces:
  ```go
  func NewTodoistApplier(port TodoistPort) domain.ProposalApplier
  func NewDispatchApplier(byOp map[string]domain.ProposalApplier, fallback domain.ProposalApplier) domain.ProposalApplier
  ```

- [ ] **Step 1: 기존 applier.go 확인**

Run: `sed -n '1,40p' internal/agent/applier.go`
기존 `HomeApplier`가 `domain.ProposalApplier`(메서드 `Apply(ctx, p) (domain.Reply, error)`)를 어떻게 구현하는지 확인(시그니처 일치 보장).

- [ ] **Step 2: 실패 테스트** — `internal/agent/applier_test.go`에 추가

```go
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
```

(import에 `"context"`, `"strings"`, `domain`, `todoist` 필요 — 파일 상단 확인.)

- [ ] **Step 3: 실패 확인**

Run: `go test ./internal/agent/ -run "Applier|Dispatch"`
Expected: FAIL — `undefined: NewTodoistApplier`

- [ ] **Step 4: 구현** — `internal/agent/applier.go`에 추가

```go
// todoistApplier 는 delete_todo 변경안을 Todoist 에 반영한다.
type todoistApplier struct {
	port TodoistPort
}

// NewTodoistApplier 는 Todoist 삭제 승인 처리기를 만든다.
func NewTodoistApplier(port TodoistPort) domain.ProposalApplier {
	return todoistApplier{port: port}
}

func (a todoistApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if p.Op != "delete_todo" {
		return domain.Reply{}, fmt.Errorf("todoistApplier: 지원하지 않는 op %q", p.Op)
	}
	if err := a.port.DeleteTask(ctx, p.Fields["task_id"]); err != nil {
		return domain.Reply{}, err
	}
	return domain.Reply{Text: "삭제했어: " + p.Fields["content"]}, nil
}

// dispatchApplier 는 ChangeProposal.Op 로 applier 를 고르고, 없으면 fallback 으로 위임한다.
type dispatchApplier struct {
	byOp     map[string]domain.ProposalApplier
	fallback domain.ProposalApplier
}

// NewDispatchApplier 는 Op 분기 applier 를 만든다.
func NewDispatchApplier(byOp map[string]domain.ProposalApplier, fallback domain.ProposalApplier) domain.ProposalApplier {
	return dispatchApplier{byOp: byOp, fallback: fallback}
}

func (a dispatchApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if ap, ok := a.byOp[p.Op]; ok {
		return ap.Apply(ctx, p)
	}
	return a.fallback.Apply(ctx, p)
}
```

`applier.go` import에 `"context"`, `"fmt"`, `domain` 있는지 확인(없으면 추가).

- [ ] **Step 5: 통과 확인**

Run: `go test ./internal/agent/ -run "Applier|Dispatch"`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/agent/applier.go internal/agent/applier_test.go
git commit -m "feat(agent): Todoist 삭제 applier + Op 분기 DispatchApplier"
```

---

### Task 5: 인프로세스 스케줄러

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

**Interfaces:**
- Produces:
  ```go
  type Job struct { Name string; Hour, Min int; TZ *time.Location; Fn func(ctx context.Context) }
  func nextFire(now time.Time, hour, min int, tz *time.Location) time.Time
  type Scheduler struct{ ... }
  func New() *Scheduler
  func (s *Scheduler) Register(j Job)
  func (s *Scheduler) Run(ctx context.Context)
  ```

- [ ] **Step 1: 실패 테스트** — `internal/scheduler/scheduler_test.go`

```go
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
```

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/scheduler/`
Expected: FAIL — `undefined: nextFire`

- [ ] **Step 3: 구현** — `internal/scheduler/scheduler.go`

```go
// Package scheduler 는 "매일 HH:MM 에 실행" 작업을 도는 인프로세스 스케줄러다.
package scheduler

import (
	"context"
	"time"

	"github.com/Jongseong0111/jarvis/pkg/log"
)

// Job 은 매일 지정 시각에 실행할 작업이다.
type Job struct {
	Name string
	Hour int
	Min  int
	TZ   *time.Location
	Fn   func(ctx context.Context)
}

// Scheduler 는 등록된 Job 들을 각자 goroutine 으로 돌린다.
type Scheduler struct {
	jobs []Job
}

// New 는 빈 스케줄러를 만든다.
func New() *Scheduler { return &Scheduler{} }

// Register 는 Job 을 등록한다(Run 전에 호출).
func (s *Scheduler) Register(j Job) { s.jobs = append(s.jobs, j) }

// Run 은 모든 Job 을 goroutine 으로 시작하고 ctx 종료까지 블록한다.
func (s *Scheduler) Run(ctx context.Context) {
	for _, j := range s.jobs {
		go s.runJob(ctx, j)
	}
	<-ctx.Done()
}

func (s *Scheduler) runJob(ctx context.Context, j Job) {
	logger := log.FromContext(ctx)
	for {
		next := nextFire(time.Now().In(j.TZ), j.Hour, j.Min, j.TZ)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.fire(ctx, j, logger)
		}
	}
}

// fire 는 Job.Fn 을 recover + 타임아웃으로 1회 실행한다.
func (s *Scheduler) fire(ctx context.Context, j Job, logger interface{ Error(string, ...any) }) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("스케줄 작업 패닉", "job", j.Name, "recover", r)
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	j.Fn(runCtx)
}

// nextFire 는 now 이후 가장 가까운 hour:min 시각을 tz 기준으로 계산한다(정각 일치면 내일).
func nextFire(now time.Time, hour, min int, tz *time.Location) time.Time {
	now = now.In(tz)
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, tz)
	if !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 1)
	}
	return candidate
}
```

> `log.FromContext`의 반환 타입 메서드 시그니처를 `sed -n '1,60p' pkg/log/log.go`로 확인. `fire`의 `logger interface{...}`는 패닉 회피용 최소 인터페이스 — 실제 로거가 `Error(msg string, args ...any)`를 가지면 그대로 통과. 안 맞으면 `*slog.Logger`로 직접 받도록 조정.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/scheduler/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/scheduler/
git commit -m "feat(scheduler): 매일 HH:MM 실행 인프로세스 스케줄러 + nextFire"
```

---

### Task 6: 브리핑 조립 + 전송 함수

**Files:**
- Create: `internal/agent/todoist_briefing.go`
- Test: `internal/agent/todoist_briefing_test.go`

**Interfaces:**
- Consumes: `TodoistPort`, `domain.MessageSender`, `todoist.Task`.
- Produces:
  ```go
  func formatBriefing(header string, tasks []todoist.Task) string
  func NewMorningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context)
  func NewEveningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context)
  ```

- [ ] **Step 1: 실패 테스트** — `internal/agent/todoist_briefing_test.go`

```go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

func TestFormatBriefing(t *testing.T) {
	t.Parallel()
	out := formatBriefing("☀️ 오늘 할일", []todoist.Task{
		{Content: "A", Due: "오늘"},
		{Content: "B"},
	})
	if !strings.Contains(out, "☀️ 오늘 할일") || !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Fatalf("out=%q", out)
	}
}

type capSender struct{ sent []domain.Reply }

func (c *capSender) Send(_ context.Context, r domain.Reply) error {
	c.sent = append(c.sent, r)
	return nil
}

func TestMorningBriefing_emptySendsNothingNote(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: nil}
	s := &capSender{}
	NewMorningBriefing(f, s, "C1")(context.Background())
	if len(s.sent) != 1 || !strings.Contains(s.sent[0].Text, "할일 없") {
		t.Fatalf("아침은 빈 날도 안내 전송: %+v", s.sent)
	}
}

func TestEveningBriefing_emptySilent(t *testing.T) {
	t.Parallel()
	f := &fakeTodoist{tasks: nil}
	s := &capSender{}
	NewEveningBriefing(f, s, "C1")(context.Background())
	if len(s.sent) != 0 {
		t.Fatalf("저녁은 빈 날 무음이어야 함: %+v", s.sent)
	}
}
```

> `domain.MessageSender` 인터페이스 시그니처를 `sed -n '1,60p' domain/slack.go`로 확인(`Send(ctx, Reply) error` 가정). `Reply`의 채널 필드명(`ChannelID`)도 확인.

- [ ] **Step 2: 실패 확인**

Run: `go test ./internal/agent/ -run Briefing`
Expected: FAIL — `undefined: formatBriefing`

- [ ] **Step 3: 구현** — `internal/agent/todoist_briefing.go`

```go
package agent

import (
	"context"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// formatBriefing 은 헤더 + 할일 목록을 텍스트로 만든다.
func formatBriefing(header string, tasks []todoist.Task) string {
	return header + "\n" + formatTaskLines(tasks)
}

// NewMorningBriefing 은 아침 브리핑 작업을 만든다(오늘+밀린, 빈 날도 안내 전송).
func NewMorningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		tasks, err := port.ListTasks(ctx, "today | overdue")
		if err != nil {
			log.FromContext(ctx).Error("아침 브리핑 조회 실패", "error", err)
			return
		}
		text := "☀️ 오늘 마감 할일이 없어. 좋은 하루!"
		if len(tasks) > 0 {
			text = formatBriefing("☀️ 오늘 할일 + 밀린 거", tasks)
		}
		send(ctx, sender, channel, text)
	}
}

// NewEveningBriefing 은 저녁 브리핑 작업을 만든다(오늘 미완료+내일, 빈 날 무음).
func NewEveningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		tasks, err := port.ListTasks(ctx, "(today & !checked) | tomorrow")
		if err != nil {
			log.FromContext(ctx).Error("저녁 브리핑 조회 실패", "error", err)
			return
		}
		if len(tasks) == 0 {
			return // 무음
		}
		send(ctx, sender, channel, formatBriefing("🌙 오늘 미완료 / 내일 할일", tasks))
	}
}

func send(ctx context.Context, sender domain.MessageSender, channel, text string) {
	if err := sender.Send(ctx, domain.Reply{ChannelID: channel, Text: text}); err != nil {
		log.FromContext(ctx).Error("브리핑 전송 실패", "error", err)
	}
}
```

> `domain.Reply` 필드명(`ChannelID`, `Text`)을 Step 1에서 확인한 값으로 맞춘다.

- [ ] **Step 4: 통과 확인**

Run: `go test ./internal/agent/ -run Briefing`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/agent/todoist_briefing.go internal/agent/todoist_briefing_test.go
git commit -m "feat(agent): Todoist 아침/저녁 브리핑 조립·전송(빈 날 아침 안내·저녁 무음)"
```

---

### Task 7: 에이전트 프롬프트 + DI 조립 + README

**Files:**
- Modify: `internal/agent/agent.go` (system 프롬프트)
- Modify: `cmd/server/main.go` (DI)
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1~6의 모든 public 심볼(`todoist.New`, `agent.TodoistTools`, `agent.NewTodoistApplier`, `agent.NewDispatchApplier`, `agent.NewMorningBriefing`, `agent.NewEveningBriefing`, `scheduler.New/Job`, `config.ParseHHMM`).

- [ ] **Step 1: system 프롬프트 추가** — `internal/agent/agent.go`

기존 system 프롬프트 문자열을 찾아(`grep -n "프롬프트\|systemPrompt\|집정리" internal/agent/agent.go`) Todoist 지침을 추가:

```
- 할일/투두 요청은 Todoist 도구를 써라. 추가/조회/완료/수정은 바로 실행하고 결과를 짧게 알려라.
- 완료/수정/삭제는 먼저 query 로 할일을 찾는다. 모호하면 되묻는다.
- 삭제는 delete_todo 로 변경안을 만들어 승인 버튼을 거친다(바로 지우지 않는다).
```

(프롬프트가 상수 문자열이면 거기에, 동적 조립이면 해당 위치에 한국어로 추가.)

- [ ] **Step 2: DI 조립** — `cmd/server/main.go`

`tools := append(...)` 다음, `ag := agent.New(...)` 이전에 Todoist 블록 삽입. 또한 applier 합성과 스케줄러 기동.

기존:
```go
	tools := append(agent.HomeTools(home, cfg.NotionHomeURL, mapURL), agent.KnowledgeTools(knowledgeSvc)...)
	ag := agent.New(geminiClient, visionClient, tools, "")
	handler := slack.NewHandler(ag, client)

	client.SetInteractionHandler(slack.NewInteractionHandler(agent.NewHomeApplier(home, renderer), client))
```

변경 후:
```go
	tools := append(agent.HomeTools(home, cfg.NotionHomeURL, mapURL), agent.KnowledgeTools(knowledgeSvc)...)

	// 변경안 적용기: 기본은 집정리, delete_todo 는 Todoist 로 분기
	var applier domain.ProposalApplier = agent.NewHomeApplier(home, renderer)

	if cfg.TodoistAPIToken != "" {
		todoistClient := todoist.New(cfg.TodoistAPIToken)
		tools = append(tools, agent.TodoistTools(todoistClient)...)
		applier = agent.NewDispatchApplier(
			map[string]domain.ProposalApplier{"delete_todo": agent.NewTodoistApplier(todoistClient)},
			applier,
		)
		if err := startBriefings(ctx, cfg, todoistClient, client, logger); err != nil {
			logger.Error("브리핑 스케줄러 시작 실패", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("Todoist 비활성(TODOIST_API_TOKEN 없음)")
	}

	ag := agent.New(geminiClient, visionClient, tools, "")
	handler := slack.NewHandler(ag, client)

	client.SetInteractionHandler(slack.NewInteractionHandler(applier, client))
```

> `ctx`는 현재 `signal.NotifyContext` 이후에 생성된다. 스케줄러는 그 `ctx`가 필요하므로 **Todoist 블록을 `ctx` 생성 이후로 옮기거나**, `ctx` 생성을 위로 올린다. 가장 단순: `ctx, stop := signal.NotifyContext(...)` 와 `ctx = log.WithContext(ctx, logger)` 두 줄을 `cfg` 로드 직후로 이동. 그 뒤 위 블록 배치.

파일 하단에 헬퍼 추가:
```go
// startBriefings 는 아침/저녁 브리핑을 스케줄러에 등록하고 백그라운드로 돌린다.
func startBriefings(ctx context.Context, cfg config.Config, client *todoist.Client, sender domain.MessageSender, logger *slog.Logger) error {
	if cfg.TodoistBriefingChannel == "" {
		logger.Info("브리핑 채널 없음 — 스케줄러 미기동(도구만 활성)")
		return nil
	}
	tz, err := time.LoadLocation(cfg.TodoistTZ)
	if err != nil {
		return fmt.Errorf("타임존 로드(%s): %w", cfg.TodoistTZ, err)
	}
	mh, mm, err := config.ParseHHMM(cfg.TodoistMorning)
	if err != nil {
		return fmt.Errorf("아침 시각: %w", err)
	}
	eh, em, err := config.ParseHHMM(cfg.TodoistEvening)
	if err != nil {
		return fmt.Errorf("저녁 시각: %w", err)
	}
	sched := scheduler.New()
	sched.Register(scheduler.Job{Name: "morning", Hour: mh, Min: mm, TZ: tz,
		Fn: agent.NewMorningBriefing(client, sender, cfg.TodoistBriefingChannel)})
	sched.Register(scheduler.Job{Name: "evening", Hour: eh, Min: em, TZ: tz,
		Fn: agent.NewEveningBriefing(client, sender, cfg.TodoistBriefingChannel)})
	go sched.Run(ctx)
	logger.Info("브리핑 스케줄러 기동", "morning", cfg.TodoistMorning, "evening", cfg.TodoistEvening, "tz", cfg.TodoistTZ)
	return nil
}
```

import 추가: `"fmt"`, `"log/slog"`, `"time"`, `"github.com/Jongseong0111/jarvis/domain"`, `"github.com/Jongseong0111/jarvis/internal/scheduler"`, `"github.com/Jongseong0111/jarvis/internal/todoist"`, `"github.com/Jongseong0111/jarvis/pkg/config"`.

> `logger`의 실제 타입을 `pkg/log`에서 확인(`log.New`가 `*slog.Logger`를 반환하는지). 아니면 `startBriefings`의 `logger` 타입을 그에 맞춘다.

- [ ] **Step 3: 빌드 + 전체 테스트**

Run: `go build ./... && go test ./... && go vet ./... && gofmt -l .`
Expected: 빌드 OK, 전체 PASS, vet 무출력, gofmt 무출력

- [ ] **Step 4: README 갱신** — `README.md`

설정 표에 행 추가:
```
| `TODOIST_API_TOKEN` | | Todoist 개인 토큰. 없으면 할일 기능 off |
| `TODOIST_BRIEFING_CHANNEL` | | 브리핑 보낼 Slack 채널/DM ID. 없으면 브리핑 off |
| `TODOIST_MORNING_TIME` | | 아침 브리핑 시각(기본 `08:00`) |
| `TODOIST_EVENING_TIME` | | 저녁 브리핑 시각(기본 `21:00`) |
| `TODOIST_BRIEFING_TZ` | | 브리핑 타임존(기본 `Asia/Seoul`) |
```

"무엇을 하나" 섹션에 한 줄 추가:
```
- **할일 (Todoist)** — "오늘 Clone Graph 풀기 추가해줘"/"오늘 할일 뭐야?"/"끝났어"로 추가·조회·완료·수정, 삭제는 승인. 아침/저녁 스케줄 브리핑.
```

로드맵에서 "할일 관리, 스케줄러(주기 알림)" 항목을 완료/갱신.

- [ ] **Step 5: 커밋**

```bash
git add internal/agent/agent.go cmd/server/main.go README.md
git commit -m "feat: Todoist 도구·삭제승인·브리핑 스케줄러 DI 조립 + 프롬프트/README"
```

---

## 수동 라이브 검증 (전체 완료 후)

`config/.env`에 `TODOIST_API_TOKEN`, `TODOIST_BRIEFING_CHANNEL` 설정 후 `go build -o bin/jarvis ./cmd/server && ./bin/jarvis`:

- [ ] "@jarvis 오늘 Clone Graph 다시 풀기 추가해줘" → Todoist 앱에 등록 + "추가했어".
- [ ] "오늘 할일 뭐야?" → 오늘+밀린 목록.
- [ ] "Clone Graph 끝났어" → 완료 처리(앱에서 체크됨).
- [ ] "Clone Graph 삭제해줘" → 승인 버튼 → 승인 시 삭제.
- [ ] 브리핑 시각을 현재+2분으로 임시 설정해 재기동 → 시각에 브리핑 DM 도착.
- [ ] `TODOIST_API_TOKEN` 주석 처리 후 재기동 → 집정리/지식 기능 정상(회귀 없음), 로그 "Todoist 비활성".
```
