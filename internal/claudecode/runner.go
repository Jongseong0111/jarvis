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
	IsError   bool   `json:"is_error"`
}

// ParseOutput 은 claude --output-format json 출력을 RunResult 로 파싱한다.
func ParseOutput(data []byte) (RunResult, error) {
	var o cliOutput
	if err := json.Unmarshal(data, &o); err != nil {
		return RunResult{}, fmt.Errorf("claude 출력 파싱 실패: %w", err)
	}
	if o.IsError {
		return RunResult{}, fmt.Errorf("claude 실행 오류: %s", o.Result)
	}
	return RunResult{SessionID: o.SessionID, Text: o.Result}, nil
}
