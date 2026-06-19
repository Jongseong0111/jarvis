package agent_test

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/agent"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// newTempGitRepo 는 테스트용 임시 git 저장소를 만들고 경로를 반환한다.
// t.Cleanup 으로 자동 삭제된다.
func newTempGitRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "jarvis-test-repo-*")
	if err != nil {
		t.Fatalf("temp dir 생성 실패: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git 명령 실패 %v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	// 빈 커밋이 없으면 브랜치 생성이 안 되므로 초기 커밋을 만든다
	run("git", "commit", "--allow-empty", "-m", "init")
	return dir
}

// fakeRunner 는 즉시 고정 결과를 반환하는 Runner 가짜 구현이다.
type fakeRunner struct {
	result claudecode.RunResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _, _ string) (claudecode.RunResult, error) {
	return f.result, f.err
}
func (f *fakeRunner) Resume(_ context.Context, _, _, _ string) (claudecode.RunResult, error) {
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

	kbPath := newTempGitRepo(t)
	port := agent.IngestPort{
		Runner:   runner,
		Registry: registry,
		Sender:   sender,
		KBPath:   kbPath,
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
