# Knowledge Phase B 구현 계획 — Claude Code 세션 슬랙 다리

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** jarvis가 knowledge-base 레포에서 Claude Code 세션을 열고, 슬랙 메시지로 ingest 시작→큐레이션→승인→PR 생성까지 이어지는 지식 개념화 흐름을 구현한다.

**Architecture:** 새 `internal/claudecode` 패키지가 `claude -p --output-format json` CLI를 실행해 RunResult(session_id, text)를 반환한다. `ReviewSessionRegistry`가 채널별 리뷰 세션 상태(session_id, branch, busy)를 관리한다. `ReviewRouter`가 `domain.MessageRouter`를 구현해 채널이 리뷰 모드면 claude --resume으로, 아니면 기존 Gemini 에이전트로 라우팅한다. 모든 claude 호출은 고루틴으로 비동기 처리하고 완료 시 Slack으로 결과를 게시한다.

**Tech Stack:** Go 1.25, `os/exec`, stdlib `encoding/json`, slack-go, google.golang.org/genai(기존), `go test -race`

## Global Constraints

- Go 컨벤션: value receiver 금지(pointer receiver 사용), constructor `New...`, 인터페이스 주입, `t.Parallel()`, 정적 시간.
- 로거: `log.FromContext(ctx)` / 고루틴 배경 컨텍스트에서 `slog.Default()`. `log.Logger()` 없음.
- 에러: `fmt.Errorf("...: %w", err)` — errors 패키지 코드젠 없음.
- 커밋: 한국어, 태스크당 1커밋.
- 테스트: fake/httptest 기반, 실제 외부 호출 없음. 라이브 검증은 마지막 태스크.
- 슬랙 응답 스타일: 존댓말(~했습니다/~해요) + 이모지 + 불릿은 `•`.
- 모든 claude 호출은 `--output-format json --permission-mode acceptEdits`.

---

### Task 1: claudecode.Runner — CLI 실행 래퍼

**Files:**
- Create: `internal/claudecode/runner.go`
- Create: `internal/claudecode/runner_test.go`

**Interfaces:**
- Produces: `claudecode.Runner` 인터페이스, `claudecode.RunResult` 구조체, `claudecode.ParseOutput([]byte) (RunResult, error)` 함수

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/claudecode/runner_test.go
package claudecode_test

import (
	"testing"

	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

func TestParseOutput_success(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"result","subtype":"success","session_id":"ses_abc","result":"개념 정리 결과입니다.","is_error":false}`)
	got, err := claudecode.ParseOutput(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "ses_abc" {
		t.Errorf("session_id: got %q, want %q", got.SessionID, "ses_abc")
	}
	if got.Text != "개념 정리 결과입니다." {
		t.Errorf("text: got %q, want %q", got.Text, "개념 정리 결과입니다.")
	}
}

func TestParseOutput_invalid(t *testing.T) {
	t.Parallel()
	_, err := claudecode.ParseOutput([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
cd ~/personal-agent/jarvis
go test ./internal/claudecode/... 2>&1
```
Expected: `cannot find package` 또는 `no Go files`

- [ ] **Step 3: 구현**

```go
// internal/claudecode/runner.go
package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// RunResult 는 claude -p 실행 결과다.
type RunResult struct {
	SessionID string
	Text      string
}

// Runner 는 Claude Code CLI 를 실행하는 능력이다(테스트에서 fake 주입).
type Runner interface {
	Run(ctx context.Context, dir, prompt string) (RunResult, error)
	Resume(ctx context.Context, sessionID, prompt string) (RunResult, error)
}

// CLIRunner 는 로컬 claude CLI 를 사용하는 Runner 구현이다.
type CLIRunner struct{}

// New 는 CLIRunner 를 만든다.
func New() *CLIRunner { return &CLIRunner{} }

// Run 은 dir 경로에서 새 claude 세션을 시작해 결과를 반환한다.
func (r *CLIRunner) Run(ctx context.Context, dir, prompt string) (RunResult, error) {
	args := []string{"-p", prompt, "--output-format", "json", "--permission-mode", "acceptEdits"}
	return r.exec(ctx, dir, args)
}

// Resume 은 기존 session_id 에 이어 메시지를 보낸다.
func (r *CLIRunner) Resume(ctx context.Context, sessionID, prompt string) (RunResult, error) {
	args := []string{"-p", prompt, "--resume", sessionID, "--output-format", "json", "--permission-mode", "acceptEdits"}
	return r.exec(ctx, "", args)
}

func (r *CLIRunner) exec(ctx context.Context, dir string, args []string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return RunResult{}, fmt.Errorf("claude 실행 실패: %w", err)
	}
	return ParseOutput(out)
}

type cliOutput struct {
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
}

// ParseOutput 은 claude --output-format json 출력을 RunResult 로 파싱한다.
func ParseOutput(data []byte) (RunResult, error) {
	var o cliOutput
	if err := json.Unmarshal(data, &o); err != nil {
		return RunResult{}, fmt.Errorf("claude 출력 파싱 실패: %w", err)
	}
	return RunResult{SessionID: o.SessionID, Text: o.Result}, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/claudecode/... -v -race
```
Expected: `PASS TestParseOutput_success`, `PASS TestParseOutput_invalid`

- [ ] **Step 5: 커밋**

```bash
git add internal/claudecode/
git commit -m "feat(claudecode): Runner 인터페이스 + CLIRunner + ParseOutput"
```

---

### Task 2: ReviewSessionRegistry — 채널별 리뷰 세션 상태

**Files:**
- Create: `internal/agent/review_session.go`
- Create: `internal/agent/review_session_test.go`

**Interfaces:**
- Produces: `agent.ReviewSession` 구조체, `agent.ReviewSessionRegistry` (Enter/Exit/Get/SetBusy/SetSessionID)

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/agent/review_session_test.go
package agent_test

import (
	"testing"

	"github.com/Jongseong0111/jarvis/internal/agent"
)

func TestReviewSessionRegistry_enterExit(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()

	_, ok := r.Get("ch1")
	if ok {
		t.Fatal("없는 채널인데 found")
	}

	r.Enter("ch1", agent.ReviewSession{Branch: "kb/ingest-foo", Slug: "foo"})
	s, ok := r.Get("ch1")
	if !ok {
		t.Fatal("Enter 후 Get 실패")
	}
	if s.Branch != "kb/ingest-foo" {
		t.Errorf("branch: got %q", s.Branch)
	}

	r.Exit("ch1")
	_, ok = r.Get("ch1")
	if ok {
		t.Fatal("Exit 후 여전히 존재")
	}
}

func TestReviewSessionRegistry_busy(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Enter("ch1", agent.ReviewSession{Slug: "foo"})

	r.SetBusy("ch1", true)
	s, _ := r.Get("ch1")
	if !s.Busy {
		t.Fatal("SetBusy(true) 후 Busy=false")
	}

	r.SetBusy("ch1", false)
	s, _ = r.Get("ch1")
	if s.Busy {
		t.Fatal("SetBusy(false) 후 Busy=true")
	}
}

func TestReviewSessionRegistry_setSessionID(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Enter("ch1", agent.ReviewSession{Slug: "foo"})
	r.SetSessionID("ch1", "ses_abc")
	s, _ := r.Get("ch1")
	if s.SessionID != "ses_abc" {
		t.Errorf("session_id: got %q", s.SessionID)
	}
}

func TestReviewSessionRegistry_exitNoop(t *testing.T) {
	t.Parallel()
	r := agent.NewReviewSessionRegistry()
	r.Exit("nonexistent") // should not panic
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/agent/... -run TestReviewSession -v 2>&1
```
Expected: `undefined: agent.ReviewSessionRegistry` 또는 compile error

- [ ] **Step 3: 구현**

```go
// internal/agent/review_session.go
package agent

import "sync"

// ReviewSession 은 채널당 활성 Claude Code 리뷰 세션이다.
type ReviewSession struct {
	SessionID  string
	Branch     string
	SourcePath string
	Slug       string
	Busy       bool
}

// ReviewSessionRegistry 는 채널별 리뷰 세션 상태를 스레드 안전하게 관리한다.
type ReviewSessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*ReviewSession
}

// NewReviewSessionRegistry 는 빈 레지스트리를 만든다.
func NewReviewSessionRegistry() *ReviewSessionRegistry {
	return &ReviewSessionRegistry{sessions: make(map[string]*ReviewSession)}
}

// Enter 는 채널을 리뷰 모드로 진입시킨다(기존 세션을 덮어씀).
func (r *ReviewSessionRegistry) Enter(channelID string, s ReviewSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := s
	r.sessions[channelID] = &cp
}

// Exit 는 채널의 리뷰 모드를 종료한다.
func (r *ReviewSessionRegistry) Exit(channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, channelID)
}

// Get 은 채널의 세션 스냅샷을 반환한다. 없으면 ok=false.
func (r *ReviewSessionRegistry) Get(channelID string) (ReviewSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[channelID]
	if !ok {
		return ReviewSession{}, false
	}
	return *s, true
}

// SetBusy 는 채널 세션의 busy 상태를 갱신한다(세션 없으면 no-op).
func (r *ReviewSessionRegistry) SetBusy(channelID string, busy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[channelID]; ok {
		s.Busy = busy
	}
}

// SetSessionID 는 비동기 Run 완료 후 session_id 를 저장한다(세션 없으면 no-op).
func (r *ReviewSessionRegistry) SetSessionID(channelID, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[channelID]; ok {
		s.SessionID = sessionID
	}
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/agent/... -run TestReviewSession -v -race
```
Expected: 4개 테스트 모두 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/agent/review_session.go internal/agent/review_session_test.go
git commit -m "feat(agent): ReviewSessionRegistry — 채널별 리뷰 세션 상태 관리"
```

---

### Task 3: channelID 컨텍스트 키 + IngestTools

**Files:**
- Modify: `internal/agent/agent.go` — channelID 컨텍스트 주입
- Create: `internal/agent/ingest_tools.go`
- Create: `internal/agent/ingest_tools_test.go`

**Interfaces:**
- Consumes: `claudecode.Runner`, `ReviewSessionRegistry`, `domain.MessageSender`
- Produces: `agent.IngestPort` 구조체, `agent.IngestTools(IngestPort) []Tool`

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/agent/ingest_tools_test.go
package agent_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// fakeRunner 는 즉시 고정 결과를 반환하는 Runner 가짜 구현이다.
type fakeRunner struct {
	result claudecode.RunResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _, _ string) (claudecode.RunResult, error) {
	return f.result, f.err
}
func (f *fakeRunner) Resume(_ context.Context, _, _ string) (claudecode.RunResult, error) {
	return f.result, f.err
}

// fakeSender 는 Send 호출을 기록하는 MessageSender 가짜 구현이다.
type fakeSender struct {
	mu      sync.Mutex
	replies []domain.Reply
}

func (f *fakeSender) Send(_ context.Context, r domain.Reply) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, r)
	return nil
}

func TestIngestTools_startConceptIngest_setsReviewMode(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses_xyz", Text: "🗂️ 개념 정리 제안"}}
	sender := &fakeSender{}
	registry := agent.NewReviewSessionRegistry()

	port := agent.IngestPort{
		Runner:   runner,
		Registry: registry,
		Sender:   sender,
		KBPath:   "/kb",
	}
	tools := agent.IngestTools(port)
	if len(tools) == 0 {
		t.Fatal("tools 비어있음")
	}
	tool := tools[0]
	if tool.Decl.Name != "start_concept_ingest" {
		t.Fatalf("tool name: %q", tool.Decl.Name)
	}

	ctx := agent.WithChannelID(context.Background(), "ch1")
	reply, err := tool.Run(ctx, map[string]any{"source_path": "sources/conversation/go-notes.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("빈 reply")
	}

	// 채널이 리뷰 모드 진입했는지 확인
	s, ok := registry.Get("ch1")
	if !ok {
		t.Fatal("리뷰 모드 진입 안 됨")
	}
	if s.Branch != "kb/ingest-go-notes" {
		t.Errorf("branch: %q", s.Branch)
	}

	// 고루틴이 session_id 저장 + slack 게시할 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, _ = registry.Get("ch1")
		if s.SessionID == "ses_xyz" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.SessionID != "ses_xyz" {
		t.Errorf("session_id 미저장: %q", s.SessionID)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.replies) == 0 {
		t.Fatal("slack 메시지 미전송")
	}
	if sender.replies[0].ChannelID != "ch1" {
		t.Errorf("channel: %q", sender.replies[0].ChannelID)
	}
	if sender.replies[0].Text != "🗂️ 개념 정리 제안" {
		t.Errorf("text: %q", sender.replies[0].Text)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/agent/... -run TestIngestTools -v 2>&1
```
Expected: compile error (WithChannelID, IngestPort 없음)

- [ ] **Step 3: agent.go에 channelID 컨텍스트 키 추가**

`internal/agent/agent.go` 파일의 패키지 레벨에(기존 상수/타입 선언 다음) 추가:

```go
// agentCtxKey 는 에이전트 내부 컨텍스트 키 타입이다.
type agentCtxKey int

const channelIDKey agentCtxKey = 1

// WithChannelID 는 channelID 를 컨텍스트에 저장한다(ingest 도구 등에서 조회).
func WithChannelID(ctx context.Context, channelID string) context.Context {
	return context.WithValue(ctx, channelIDKey, channelID)
}

// channelIDFromCtx 는 컨텍스트에서 channelID 를 꺼낸다(없으면 "").
func channelIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(channelIDKey).(string)
	return v
}
```

`Route` 메서드의 첫 줄에 삽입 (이미지 처리 블록 위):

```go
func (a Agent) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	ctx = WithChannelID(ctx, in.ChannelID)  // ← 추가
	if len(in.Images) > 0 && a.vision != nil {
```

- [ ] **Step 4: ingest_tools.go 구현**

```go
// internal/agent/ingest_tools.go
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
	"google.golang.org/genai"
)

// IngestPort 는 start_concept_ingest 도구가 필요로 하는 의존성이다.
type IngestPort struct {
	Runner   claudecode.Runner
	Registry *ReviewSessionRegistry
	Sender   domain.MessageSender
	KBPath   string
}

type ingestTools struct {
	port IngestPort
}

// IngestTools 는 start_concept_ingest 도구 목록을 만든다.
func IngestTools(port IngestPort) []Tool {
	k := &ingestTools{port: port}
	return []Tool{k.startConceptIngest()}
}

func (k *ingestTools) startConceptIngest() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "start_concept_ingest",
			Description: "저장된 소스 파일을 개념 문서로 정리하는 Claude Code 세션을 시작한다. 완료 결과(개념 트리)는 이 채널에 게시된다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"source_path": strSchema("정리할 소스 파일 경로(knowledge-base 기준 상대경로, 예: sources/conversation/go-notes.md)"),
			}, "source_path"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			channelID := channelIDFromCtx(ctx)
			if channelID == "" {
				return "", fmt.Errorf("채널 ID 없음")
			}
			sourcePath := strArg(args, "source_path")
			slug := slugFrom(sourcePath)
			branch := "kb/ingest-" + slug

			// 이미 리뷰 모드면 중복 시작 방지
			if _, ok := k.port.Registry.Get(channelID); ok {
				return "이미 개념 정리 세션이 진행 중이에요. 완료 후 다시 시도해주세요.", nil
			}

			k.port.Registry.Enter(channelID, ReviewSession{
				Branch:     branch,
				SourcePath: sourcePath,
				Slug:       slug,
				Busy:       true,
			})

			prompt := fmt.Sprintf(
				"/kb-ingest %s --type=conversation\n끝나면 제안 개념 트리 + 각 개념 1줄 요약을 슬랙용으로 정리해서 보여줘. (이모지 분류, 드롭 항목 포함)",
				sourcePath,
			)

			go func() {
				bgCtx := context.Background()
				result, err := k.port.Runner.Run(bgCtx, k.port.KBPath, prompt)
				if err != nil {
					slog.Default().Error("ingest 실패", "channel", channelID, "error", err)
					_ = k.port.Sender.Send(bgCtx, domain.Reply{
						ChannelID: channelID,
						Text:      "🚨 개념 정리 중 문제가 생겼어요. 다시 시도해볼까요?",
					})
					k.port.Registry.Exit(channelID)
					return
				}
				k.port.Registry.SetSessionID(channelID, result.SessionID)
				k.port.Registry.SetBusy(channelID, false)
				_ = k.port.Sender.Send(bgCtx, domain.Reply{
					ChannelID: channelID,
					Text:      result.Text,
				})
			}()

			return "🧠 개념 정리를 시작했습니다. 잠시 기다려주세요...", nil
		},
	}
}

// slugFrom 은 소스 파일 경로에서 브랜치/slug 이름을 만든다.
func slugFrom(sourcePath string) string {
	base := filepath.Base(sourcePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
```

- [ ] **Step 5: 테스트 통과 확인**

```bash
go test ./internal/agent/... -run TestIngestTools -v -race
```
Expected: PASS

- [ ] **Step 6: 전체 테스트 확인**

```bash
go test ./... -race
```
Expected: 모두 PASS

- [ ] **Step 7: 커밋**

```bash
git add internal/agent/agent.go internal/agent/ingest_tools.go internal/agent/ingest_tools_test.go
git commit -m "feat(agent): channelID 컨텍스트 키 + start_concept_ingest 도구"
```

---

### Task 4: ReviewRouter — 채널 모드별 라우팅

**Files:**
- Create: `internal/agent/review_router.go`
- Create: `internal/agent/review_router_test.go`

**Interfaces:**
- Consumes: `domain.MessageRouter`(base agent), `ReviewSessionRegistry`, `claudecode.Runner`, `domain.MessageSender`
- Produces: `agent.ReviewRouter` — `domain.MessageRouter` 구현

- [ ] **Step 1: 실패 테스트 작성**

```go
// internal/agent/review_router_test.go
package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// fakeBaseRouter 는 기록하는 MessageRouter 가짜 구현이다.
type fakeBaseRouter struct {
	called int
	reply  domain.Reply
}

func (f *fakeBaseRouter) Route(_ context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	f.called++
	f.reply.ChannelID = in.ChannelID
	f.reply.Text = "base:" + in.Text
	return f.reply, nil
}

func TestReviewRouter_notInReview_delegatesToBase(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{}
	sender := &fakeSender{}

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "안녕"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply.Text != "base:안녕" {
		t.Errorf("text: %q", reply.Text)
	}
	if base.called != 1 {
		t.Errorf("base not called")
	}
}

func TestReviewRouter_inReview_busy_returnsBusyMessage(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Busy: true, Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "channel 빼"})
	if err != nil {
		t.Fatal(err)
	}
	if base.called != 0 {
		t.Error("busy 중엔 base 호출 안 해야 함")
	}
	if reply.Text == "" {
		t.Error("busy 안내 메시지 없음")
	}
}

func TestReviewRouter_inReview_cancel_exitsMode(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "ok"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Branch: "kb/ingest-foo", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "취소"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text == "" {
		t.Error("취소 안내 없음")
	}
	_, still := registry.Get("ch1")
	if still {
		t.Error("취소 후 리뷰 모드 해제 안 됨")
	}
}

func TestReviewRouter_inReview_curate_sendsAsyncAndReturnsPending(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "수정됨"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	reply, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "channel 빼"})
	if err != nil {
		t.Fatal(err)
	}
	// 동기 응답은 "처리 중" 안내여야 함
	if reply.ChannelID != "ch1" {
		t.Errorf("channel: %q", reply.ChannelID)
	}
	if reply.Text == "" {
		t.Error("처리 중 안내 없음")
	}

	// 고루틴이 slack 에 결과 게시할 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sender.mu.Lock()
		n := len(sender.replies)
		sender.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.replies) == 0 {
		t.Fatal("고루틴 slack 전송 없음")
	}
	if sender.replies[0].Text != "수정됨" {
		t.Errorf("async text: %q", sender.replies[0].Text)
	}
}

func TestReviewRouter_inReview_approve_exitsAndSendsAsync(t *testing.T) {
	t.Parallel()
	base := &fakeBaseRouter{}
	registry := agent.NewReviewSessionRegistry()
	runner := &fakeRunner{result: claudecode.RunResult{SessionID: "ses1", Text: "PR 생성됨"}}
	sender := &fakeSender{}

	registry.Enter("ch1", agent.ReviewSession{SessionID: "ses1", Branch: "kb/ingest-foo", Slug: "foo"})

	rr := agent.NewReviewRouter(base, registry, runner, sender)
	_, err := rr.Route(context.Background(), domain.IncomingMessage{ChannelID: "ch1", Text: "승인"})
	if err != nil {
		t.Fatal(err)
	}

	// 고루틴 완료 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sender.mu.Lock()
		n := len(sender.replies)
		sender.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 승인 후 리뷰 모드 해제
	_, still := registry.Get("ch1")
	if still {
		t.Error("승인 후 리뷰 모드 해제 안 됨")
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.replies) == 0 {
		t.Fatal("PR 결과 전송 없음")
	}
}
```

(이 파일에서 `time` import를 추가해야 함)

- [ ] **Step 2: 테스트 실패 확인**

```bash
go test ./internal/agent/... -run TestReviewRouter -v 2>&1
```
Expected: compile error (NewReviewRouter 없음)

- [ ] **Step 3: 구현**

```go
// internal/agent/review_router.go
package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// ReviewRouter 는 채널이 리뷰 모드일 때 claude 세션으로, 아닐 때 기본 에이전트로 라우팅한다.
// domain.MessageRouter 를 구현한다.
type ReviewRouter struct {
	base     domain.MessageRouter
	registry *ReviewSessionRegistry
	runner   claudecode.Runner
	sender   domain.MessageSender
}

// NewReviewRouter 는 ReviewRouter 를 만든다.
func NewReviewRouter(base domain.MessageRouter, registry *ReviewSessionRegistry, runner claudecode.Runner, sender domain.MessageSender) *ReviewRouter {
	return &ReviewRouter{base: base, registry: registry, runner: runner, sender: sender}
}

// Route 는 채널 상태에 따라 메시지를 라우팅한다.
func (r *ReviewRouter) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	session, inReview := r.registry.Get(in.ChannelID)
	if !inReview {
		return r.base.Route(ctx, in)
	}

	if session.Busy {
		return domain.Reply{ChannelID: in.ChannelID, Text: "⏳ 아직 처리 중이에요. 잠시 후 말씀해주세요."}, nil
	}

	text := strings.TrimSpace(in.Text)

	if isCancelKeyword(text) {
		r.registry.Exit(in.ChannelID)
		return domain.Reply{
			ChannelID: in.ChannelID,
			Text:      "❌ 개념 정리를 취소했습니다. 브랜치(" + session.Branch + ")는 남아있어요.",
		}, nil
	}

	if isApproveKeyword(text) {
		return r.approve(in.ChannelID, session)
	}

	return r.curate(in.ChannelID, session, text)
}

func (r *ReviewRouter) curate(channelID string, session ReviewSession, text string) (domain.Reply, error) {
	r.registry.SetBusy(channelID, true)
	go func() {
		bgCtx := context.Background()
		result, err := r.runner.Resume(bgCtx, session.SessionID, text)
		r.registry.SetBusy(channelID, false)
		if err != nil {
			slog.Default().Error("curate resume 실패", "channel", channelID, "error", err)
			_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: "🚨 처리 중 문제가 생겼어요."})
			return
		}
		_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: result.Text})
	}()
	return domain.Reply{ChannelID: channelID, Text: "⏳ 처리 중입니다..."}, nil
}

func (r *ReviewRouter) approve(channelID string, session ReviewSession) (domain.Reply, error) {
	r.registry.SetBusy(channelID, true)
	approvePrompt := "/kb-approve 한 뒤, 이 브랜치를 push 하고 gh 로 main 대상 PR 을 생성해줘."
	go func() {
		bgCtx := context.Background()
		result, err := r.runner.Resume(bgCtx, session.SessionID, approvePrompt)
		r.registry.Exit(channelID)
		if err != nil {
			slog.Default().Error("approve 실패", "channel", channelID, "error", err)
			_ = r.sender.Send(bgCtx, domain.Reply{
				ChannelID: channelID,
				Text:      "🚨 승인 처리 중 문제가 생겼어요. 브랜치(" + session.Branch + ")는 남아있어요.",
			})
			return
		}
		_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: result.Text})
	}()
	return domain.Reply{ChannelID: channelID, Text: "✅ 승인 처리 중입니다..."}, nil
}

var cancelKeywords = []string{"취소", "그만", "cancel", "stop"}
var approveKeywords = []string{"승인", "이대로", "approve"}

func isCancelKeyword(s string) bool {
	for _, kw := range cancelKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func isApproveKeyword(s string) bool {
	for _, kw := range approveKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 테스트 통과 확인**

```bash
go test ./internal/agent/... -run TestReviewRouter -v -race
```
Expected: 5개 테스트 모두 PASS

- [ ] **Step 5: 전체 테스트**

```bash
go test ./... -race
```
Expected: 모두 PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/agent/review_router.go internal/agent/review_router_test.go
git commit -m "feat(agent): ReviewRouter — 리뷰 모드 채널 claude 세션 라우팅"
```

---

### Task 5: main.go 조립 + 시스템 프롬프트 업데이트

**Files:**
- Modify: `cmd/server/main.go` — ReviewRouter + IngestTools 조립
- Modify: `internal/agent/agent.go` DefaultSystemPrompt — start_concept_ingest 지시 추가

**Interfaces:**
- Consumes: `claudecode.New()`, `agent.NewReviewSessionRegistry()`, `agent.IngestTools()`, `agent.NewReviewRouter()`

- [ ] **Step 1: DefaultSystemPrompt에 지시 추가**

`internal/agent/agent.go`의 `DefaultSystemPrompt` 상수 끝(마지막 줄 backtick 바로 앞)에 아래 규칙을 추가:

```go
const DefaultSystemPrompt = `...기존 내용...
- save_kb_source 로 소스를 저장한 직후, "저장했습니다! 이 내용을 개념 문서로 정리할까요? 🗂️" 라고 제안해라. 사용자가 "응/그래/해줘/해줘봐" 등 긍정 답변을 하면 start_concept_ingest 를 호출하되, source_path 에는 save_kb_source 가 반환한 경로(예: sources/conversation/xxx.md)를 넣어라.
- 사용자가 명시적으로 "개념 정리해줘", "지식 정리해줘" 등을 요청하면 start_concept_ingest 를 호출한다. source_path 가 언급되지 않으면 직전 대화에서 저장된 경로를 쓴다.`
```

- [ ] **Step 2: 빌드 확인 (변경 전)**

```bash
cd ~/personal-agent/jarvis && go build ./...
```
Expected: PASS

- [ ] **Step 3: main.go 수정**

`cmd/server/main.go`의 import에 추가:
```go
"github.com/Jongseong0111/jarvis/internal/claudecode"
```

`tools = append(tools, agent.KnowledgeTools(knowledgeSvc)...)` 줄 **다음에** 삽입:

```go
// Phase B: knowledge ingest — Claude Code 세션 다리
ccRunner := claudecode.New()
reviewRegistry := agent.NewReviewSessionRegistry()
ingestPort := agent.IngestPort{
    Runner:   ccRunner,
    Registry: reviewRegistry,
    Sender:   client,
    KBPath:   cfg.KnowledgeRepoPath,
}
tools = append(tools, agent.IngestTools(ingestPort)...)
```

`ag := agent.New(...)` 줄 다음에 **현재 handler 생성 전에** 삽입:

```go
reviewRouter := agent.NewReviewRouter(ag, reviewRegistry, ccRunner, client)
handler := slack.NewHandler(reviewRouter, client)
```

기존 `handler := slack.NewHandler(ag, client)` 줄 **삭제**.

- [ ] **Step 4: 빌드 확인**

```bash
go build ./...
```
Expected: PASS (에러 없음)

- [ ] **Step 5: 전체 테스트**

```bash
go test ./... -race
```
Expected: 모두 PASS

- [ ] **Step 6: 커밋**

```bash
git add cmd/server/main.go internal/agent/agent.go
git commit -m "feat(server): Phase B 조립 — ReviewRouter + IngestTools 연결"
```

---

### Task 6: 라이브 검증 + 브랜치 push

**Files:**
- 신규 파일 없음 — 서버 재시작 후 슬랙 E2E

- [ ] **Step 1: 서버 빌드 및 재시작**

```bash
cd ~/personal-agent/jarvis
go build -o bin/jarvis ./cmd/server
pkill -f bin/jarvis || true
nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
echo "PID: $!"
```

- [ ] **Step 2: 기존 기능 회귀 확인**

슬랙에서 다음을 확인(모두 정상이어야 함):
- "안녕" → 자연 인사 응답
- "건전지 어디있어?" → Notion 검색 결과
- "오늘 할일 뭐야?" → Todoist 목록

- [ ] **Step 3: 개념 정리 트리거 — save 직후 제안 경로**

슬랙에서:
1. ChatGPT 공유 링크 + "정리해줘" → summarize 응답 확인
2. "저장해" → save_kb_source 호출 → "저장했습니다! 개념 정리할까요?" 제안 확인
3. "응" → start_concept_ingest 호출 → "정리 중..." 응답 확인
4. (1-3분 대기) → 개념 트리 슬랙 게시 확인
5. "channel 빼" → "처리 중..." + 수정된 트리 확인
6. "승인" → "승인 처리 중..." + PR 링크 확인
7. GitHub에서 PR 내용 확인

- [ ] **Step 4: 취소 경로 확인**

1. 다른 소스 개념 정리 시작 후 "취소" → 리뷰 모드 해제 + 브랜치 남음 안내 확인

- [ ] **Step 5: 로그 확인**

```bash
tail -50 /tmp/jarvis.log
```
Expected: 에러 없음, 정상 흐름 로그

- [ ] **Step 6: feature 브랜치 push + PR**

```bash
cd ~/personal-agent/jarvis
git checkout -b feature/knowledge-phase-b
git push -u origin feature/knowledge-phase-b
gh pr create \
  --title "feat: knowledge Phase B — Claude Code 세션 슬랙 다리" \
  --body "$(cat <<'EOF'
## Summary
- \`internal/claudecode\`: \`claude -p --output-format json\` CLI 래퍼 (Runner 인터페이스 + CLIRunner + ParseOutput)
- \`internal/agent/ReviewSessionRegistry\`: 채널별 리뷰 세션 상태 관리 (Enter/Exit/Busy/SessionID)
- \`internal/agent/IngestTools\`: \`start_concept_ingest\` 도구 — goroutine 비동기 + Slack 게시
- \`internal/agent/ReviewRouter\`: 리뷰 모드 채널은 \`claude --resume\`, 아니면 Gemini 에이전트
- 시스템 프롬프트: save 직후 개념정리 제안 지시 추가

## Test plan
- [ ] \`go test ./... -race\` 모두 PASS
- [ ] 슬랙 E2E: save → 제안 → "응" → 개념 트리 게시
- [ ] 큐레이션("channel 빼") → 수정 트리 게시
- [ ] "승인" → PR 링크 게시
- [ ] "취소" → 리뷰 모드 해제
- [ ] 기존 기능(집정리·Todoist) 회귀 없음

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
