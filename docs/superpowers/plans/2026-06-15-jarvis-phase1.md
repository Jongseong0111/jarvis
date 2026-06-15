# Jarvis Phase 1 (구조 재편 + Slack echo) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 개인 프로젝트를 `~/personal-agent/`로 분리(개인 GitHub 계정 스코핑)하고, Slack 멘션/DM을 받아 echo 응답하는 jarvis Go 서버(Phase 1)를 구현한다.

**Architecture:** acloset-api의 Clean Architecture(Domain→Channel) + constructor 주입 컨벤션을 규모에 맞게 차용. Slack은 Socket Mode(로컬, public URL 불필요). 핵심 변환(`buildEcho`)은 순수함수로 분리해 TDD.

**Tech Stack:** Go, `slack-go/slack`(+socketmode), `joho/godotenv`, stdlib `log/slog`.

설계 출처: `2026-06-15-jarvis-phase1-design.md`.

---

## 파일 구조 (jarvis repo)

```txt
jarvis/
  cmd/server/main.go            # 진입점 + DI 조립
  domain/slack.go               # IncomingMessage/Reply DTO + MessageSender 인터페이스
  internal/slack/
    client.go                   # Socket Mode 연결/이벤트 루프 + 전송(MessageSender 구현)
    handler.go                  # IncomingMessage → echo 변환/전송
    handler_test.go             # buildEcho 단위테스트
  pkg/config/config.go          # Config struct + New() + 검증
  pkg/config/config_test.go     # validate 단위테스트
  pkg/log/log.go                # slog 로거 + 컨텍스트 주입
  config/.env.example           # 토큰 템플릿
  Makefile
  .golangci.yaml
  .mockery.yaml
  .gitignore
  go.mod
  CLAUDE.md
  docs/superpowers/{specs,plans}/
```

> **사용자 액션이 필요한 단계는 `🧑 USER ACTION`으로 표시.** 해당 단계는 사용자가 직접 수행한 뒤 진행한다.

---

## Task 1: 개인 git identity + SSH 키 셋업

**Files:**
- Create: `~/.gitconfig-personal`
- Modify: `~/.gitconfig`
- Create: `~/.ssh/id_ed25519_personal`(+`.pub`)

- [ ] **Step 1: 개인 SSH 키 생성**

Run:
```bash
ssh-keygen -t ed25519 -C "leon1111@naver.com" -f ~/.ssh/id_ed25519_personal -N ""
```
Expected: `~/.ssh/id_ed25519_personal` 와 `.pub` 생성.

- [ ] **Step 2: 개인 gitconfig 작성**

Create `~/.gitconfig-personal`:
```ini
[user]
    name = Jongseong0111
    email = leon1111@naver.com
[core]
    sshCommand = ssh -i ~/.ssh/id_ed25519_personal -o IdentitiesOnly=yes
```

- [ ] **Step 3: global gitconfig에 includeIf 추가**

`~/.gitconfig` 끝에 추가 (기존 내용은 보존):
```ini
[includeIf "gitdir:~/personal-agent/"]
    path = ~/.gitconfig-personal
```

- [ ] **Step 4: 공개키 출력 → 🧑 USER ACTION (GitHub 등록)**

Run:
```bash
cat ~/.ssh/id_ed25519_personal.pub
```
출력 내용을 GitHub(개인계정) → Settings → SSH and GPG keys → New SSH key 에 등록.

- [ ] **Step 5: 🧑 USER ACTION — 원격 repo 생성**

GitHub 개인계정에서 빈 repo 두 개 생성(README 없이):
- `Jongseong0111/jarvis`
- `Jongseong0111/knowledge-base`

---

## Task 2: `~/personal-agent/` 생성 + knowledge-base 이동

**Files:**
- Move: `~/acloset-agent/base/knowledge-base/` → `~/personal-agent/knowledge-base/`
- Move: KB 설계/플랜 문서 → `~/personal-agent/knowledge-base/docs/specs/`

- [ ] **Step 1: 작업공간 폴더 생성**

Run:
```bash
mkdir -p ~/personal-agent
```

- [ ] **Step 2: knowledge-base repo 이동**

Run:
```bash
mv ~/acloset-agent/base/knowledge-base ~/personal-agent/knowledge-base
```

- [ ] **Step 3: KB 설계/플랜 문서 이동**

Run:
```bash
mkdir -p ~/personal-agent/knowledge-base/docs/specs
mv ~/acloset-agent/base/docs/superpowers/specs/2026-05-14-knowledge-base-design.md ~/personal-agent/knowledge-base/docs/specs/
mv ~/acloset-agent/base/docs/superpowers/plans/2026-05-14-knowledge-base-mvp1.md ~/personal-agent/knowledge-base/docs/specs/
```

- [ ] **Step 4: git identity 적용 확인 (includeIf 동작 검증)**

Run:
```bash
git -C ~/personal-agent/knowledge-base config user.email
```
Expected: `leon1111@naver.com` (회사 이메일이 아니면 Task 1 includeIf 경로 확인)

- [ ] **Step 5: 원격 연결 + 문서 이동 커밋**

Run:
```bash
git -C ~/personal-agent/knowledge-base remote set-url origin git@github.com:Jongseong0111/knowledge-base.git 2>/dev/null || git -C ~/personal-agent/knowledge-base remote add origin git@github.com:Jongseong0111/knowledge-base.git
git -C ~/personal-agent/knowledge-base add docs/
git -C ~/personal-agent/knowledge-base commit -m "docs: 설계/MVP 플랜 문서를 repo 내부로 이동

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```
Expected: 커밋 생성. (push는 Task 14에서 사용자 승인 후)

---

## Task 3: jarvis repo 스캐폴드

**Files:**
- Create: `~/personal-agent/jarvis/` 디렉터리 트리, `go.mod`, `.gitignore`
- Move: `CLAUDE.md`, 설계 spec, 이 플랜 문서 → jarvis 내부

- [ ] **Step 1: 디렉터리 트리 생성**

Run:
```bash
mkdir -p ~/personal-agent/jarvis/{cmd/server,domain,internal/slack,pkg/config,pkg/log,config,docs/superpowers/specs,docs/superpowers/plans}
```

- [ ] **Step 2: go module 초기화**

Run:
```bash
cd ~/personal-agent/jarvis && go mod init github.com/Jongseong0111/jarvis
```
Expected: `go.mod` 생성 (module github.com/Jongseong0111/jarvis).

- [ ] **Step 3: CLAUDE.md + 문서 이동**

Run:
```bash
mv ~/acloset-agent/base/CLAUDE.md ~/personal-agent/jarvis/CLAUDE.md
mv ~/acloset-agent/base/docs/superpowers/specs/2026-06-15-jarvis-phase1-design.md ~/personal-agent/jarvis/docs/superpowers/specs/
mv ~/acloset-agent/base/docs/superpowers/plans/2026-06-15-jarvis-phase1.md ~/personal-agent/jarvis/docs/superpowers/plans/
```
(CLAUDE.md는 Task 13에서 내용 갱신)

- [ ] **Step 4: .gitignore 작성**

Create `~/personal-agent/jarvis/.gitignore`:
```gitignore
# secrets
config/.env
.env

# build
/bin/
*.test
*.out

# os/editor
.DS_Store
.idea/
```

- [ ] **Step 5: git init + identity 확인 + 초기 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git init
git remote add origin git@github.com:Jongseong0111/jarvis.git
git config user.email
```
Expected: `git config user.email` → `leon1111@naver.com`.

```bash
git add .
git commit -m "chore: jarvis repo 스캐폴드 + 설계/플랜 문서

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: 컨벤션 베이스라인 파일

**Files:**
- Create: `Makefile`, `.golangci.yaml`, `.mockery.yaml`, `config/.env.example`

- [ ] **Step 1: Makefile 작성**

Create `~/personal-agent/jarvis/Makefile`:
```makefile
.PHONY: run test lint mock tidy

run:
	go run ./cmd/server

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

mock:
	mockery

tidy:
	go mod tidy
```

- [ ] **Step 2: .golangci.yaml 작성 (acloset-api 축약본)**

Create `~/personal-agent/jarvis/.golangci.yaml`:
```yaml
# acloset-api 컨벤션 축약본. 설치된 golangci-lint 버전에 맞춰 필요시 조정.
linters:
  disable-all: true
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - paralleltest
    - wrapcheck
```

- [ ] **Step 3: .mockery.yaml 작성 (후속 Phase 대비)**

Create `~/personal-agent/jarvis/.mockery.yaml`:
```yaml
# Phase 2+ Worker 인터페이스 mock 생성용. Phase 1에선 사용 안 함.
with-expecter: true
dir: "{{.InterfaceDir}}/mocks"
filename: "{{.InterfaceName}}.go"
mockname: "{{.InterfaceName}}"
packages:
  github.com/Jongseong0111/jarvis/domain:
    config:
      all: true
```

- [ ] **Step 4: config/.env.example 작성**

Create `~/personal-agent/jarvis/config/.env.example`:
```env
# 복사해서 config/.env 로 채워 사용 (config/.env 는 gitignore됨)
SLACK_BOT_TOKEN=
SLACK_APP_TOKEN=
```

- [ ] **Step 5: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add Makefile .golangci.yaml .mockery.yaml config/.env.example
git commit -m "chore: 컨벤션 베이스라인(Makefile/lint/mock/env) 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: pkg/log — slog 로거

**Files:**
- Create: `pkg/log/log.go`
- Test: `pkg/log/log_test.go`

- [ ] **Step 1: 실패 테스트 작성**

Create `~/personal-agent/jarvis/pkg/log/log_test.go`:
```go
package log

import (
	"context"
	"testing"
)

func TestFromContext_기본로거_폴백(t *testing.T) {
	t.Parallel()
	if got := FromContext(context.Background()); got == nil {
		t.Fatal("FromContext 가 nil 을 반환하면 안 됨")
	}
}

func TestWithContext_라운드트립(t *testing.T) {
	t.Parallel()
	logger := New("local")
	ctx := WithContext(context.Background(), logger)
	if got := FromContext(ctx); got != logger {
		t.Fatal("FromContext 가 주입한 로거를 반환해야 함")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/log/ -run TestFromContext`
Expected: 컴파일 실패 (`undefined: FromContext`).

- [ ] **Step 3: 최소 구현**

Create `~/personal-agent/jarvis/pkg/log/log.go`:
```go
// Package log 는 slog 기반 구조화 로거와 컨텍스트 전달을 제공한다.
package log

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

// New 는 환경에 맞는 로거를 만든다. local 은 사람이 읽기 쉬운 text, 그 외는 JSON.
func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler
	if env == "local" || env == "" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// WithContext 는 로거를 컨텍스트에 싣는다.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext 는 컨텍스트의 로거를 반환한다. 없으면 기본 로거.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/log/`
Expected: PASS.

- [ ] **Step 5: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add pkg/log/
git commit -m "feat: slog 기반 구조화 로거 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: pkg/config — 설정 로드/검증

**Files:**
- Create: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

- [ ] **Step 1: 실패 테스트 작성**

Create `~/personal-agent/jarvis/pkg/config/config_test.go`:
```go
package config

import "testing"

func TestConfig_validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "정상", cfg: Config{SlackBotToken: "xoxb-1", SlackAppToken: "xapp-1"}, wantErr: false},
		{name: "bot 토큰 누락", cfg: Config{SlackAppToken: "xapp-1"}, wantErr: true},
		{name: "app 토큰 누락", cfg: Config{SlackBotToken: "xoxb-1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/`
Expected: 컴파일 실패 (`undefined: Config`).

- [ ] **Step 3: 최소 구현**

Create `~/personal-agent/jarvis/pkg/config/config.go`:
```go
// Package config 는 .env/환경변수에서 jarvis 설정을 로드하고 검증한다.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config 는 jarvis 실행에 필요한 설정값이다.
type Config struct {
	Env           string
	SlackBotToken string
	SlackAppToken string
}

// New 는 config/.env(있으면)와 환경변수에서 설정을 로드하고 필수값을 검증한다.
func New() (Config, error) {
	_ = godotenv.Load("config/.env") // 파일 없으면 무시 (환경변수만으로도 동작)

	cfg := Config{
		Env:           getenv("JARVIS_ENV", "local"),
		SlackBotToken: os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken: os.Getenv("SLACK_APP_TOKEN"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.SlackBotToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN 이 비어있습니다")
	}
	if c.SlackAppToken == "" {
		return fmt.Errorf("SLACK_APP_TOKEN 이 비어있습니다")
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 4: 의존성 받기 + 테스트 통과 확인**

Run:
```bash
cd ~/personal-agent/jarvis
go get github.com/joho/godotenv
go test ./pkg/config/
```
Expected: PASS.

- [ ] **Step 5: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add pkg/config/ go.mod go.sum
git commit -m "feat: .env/환경변수 기반 Config 로드/검증 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: domain/slack.go — DTO + 인터페이스

**Files:**
- Create: `domain/slack.go`

- [ ] **Step 1: 도메인 타입 작성**

Create `~/personal-agent/jarvis/domain/slack.go`:
```go
// Package domain 은 채널 독립적인 인터페이스/DTO 를 정의한다(구현 없음).
package domain

import "context"

// IncomingMessage 는 채널에서 수신한 사용자 메시지다.
type IncomingMessage struct {
	ChannelID string
	UserID    string
	Text      string
}

// Reply 는 채널로 보낼 응답이다.
type Reply struct {
	ChannelID string
	Text      string
}

// MessageSender 는 채널로 메시지를 전송하는 능력이다.
type MessageSender interface {
	Send(ctx context.Context, reply Reply) error
}
```

- [ ] **Step 2: 빌드 확인**

Run: `cd ~/personal-agent/jarvis && go build ./domain/`
Expected: 성공 (출력 없음).

- [ ] **Step 3: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add domain/
git commit -m "feat: 채널 도메인 타입(IncomingMessage/Reply/MessageSender) 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: internal/slack/handler.go — echo 변환 (TDD)

**Files:**
- Create: `internal/slack/handler.go`
- Test: `internal/slack/handler_test.go`

- [ ] **Step 1: 실패 테스트 작성**

Create `~/personal-agent/jarvis/internal/slack/handler_test.go`:
```go
package slack

import (
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func Test_buildEcho(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       domain.IncomingMessage
		wantText string
		wantOK   bool
	}{
		{
			name:     "멘션 토큰 제거 후 echo",
			in:       domain.IncomingMessage{ChannelID: "C1", Text: "<@U123> 안녕"},
			wantText: "안녕",
			wantOK:   true,
		},
		{
			name:     "DM 평문 echo",
			in:       domain.IncomingMessage{ChannelID: "D1", Text: "건전지 어디 뒀지?"},
			wantText: "건전지 어디 뒀지?",
			wantOK:   true,
		},
		{
			name:   "멘션만 있고 본문 없음 → 응답 안 함",
			in:     domain.IncomingMessage{ChannelID: "C1", Text: "<@U123>   "},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reply, ok := buildEcho(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("buildEcho ok = %v, want %v", ok, tt.wantOK)
			}
			if ok {
				if reply.Text != tt.wantText {
					t.Fatalf("buildEcho text = %q, want %q", reply.Text, tt.wantText)
				}
				if reply.ChannelID != tt.in.ChannelID {
					t.Fatalf("buildEcho channel = %q, want %q", reply.ChannelID, tt.in.ChannelID)
				}
			}
		})
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/slack/`
Expected: 컴파일 실패 (`undefined: buildEcho`).

- [ ] **Step 3: 최소 구현**

Create `~/personal-agent/jarvis/internal/slack/handler.go`:
```go
// Package slack 은 Slack 채널 어댑터(연결/이벤트 처리/전송)를 구현한다.
package slack

import (
	"context"
	"regexp"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// mentionPattern 은 Slack 멘션 토큰(<@U123>)을 매칭한다.
var mentionPattern = regexp.MustCompile(`<@[A-Z0-9]+>`)

// Handler 는 수신 메시지를 echo 응답으로 처리한다.
type Handler struct {
	sender domain.MessageSender
}

// NewHandler 는 Handler 를 생성한다.
func NewHandler(sender domain.MessageSender) Handler {
	return Handler{sender: sender}
}

// Handle 은 수신 메시지를 echo 로 변환해 전송한다.
func (h Handler) Handle(ctx context.Context, in domain.IncomingMessage) error {
	reply, ok := buildEcho(in)
	if !ok {
		return nil
	}
	log.FromContext(ctx).Info("echo 응답", "channel", reply.ChannelID, "text", reply.Text)
	return h.sender.Send(ctx, reply)
}

// buildEcho 는 수신 메시지로부터 echo 응답을 계산한다. SDK/네트워크 비의존.
func buildEcho(in domain.IncomingMessage) (domain.Reply, bool) {
	text := strings.TrimSpace(mentionPattern.ReplaceAllString(in.Text, ""))
	if text == "" {
		return domain.Reply{}, false
	}
	return domain.Reply{ChannelID: in.ChannelID, Text: text}, true
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/slack/`
Expected: PASS.

- [ ] **Step 5: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add internal/slack/handler.go internal/slack/handler_test.go
git commit -m "feat: 멘션/DM echo 변환 핸들러 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: internal/slack/client.go — Socket Mode 연결

**Files:**
- Create: `internal/slack/client.go`

- [ ] **Step 1: 클라이언트 구현 작성**

Create `~/personal-agent/jarvis/internal/slack/client.go`:
```go
package slack

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
	slackgo "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Client 는 Slack Socket Mode 연결을 관리하고 메시지를 송수신한다.
// domain.MessageSender 를 구현한다.
type Client struct {
	api    *slackgo.Client
	socket *socketmode.Client
	botID  string
}

// NewClient 는 봇/앱 토큰으로 Socket Mode 클라이언트를 생성한다.
func NewClient(botToken, appToken string) (*Client, error) {
	api := slackgo.New(botToken, slackgo.OptionAppLevelToken(appToken))
	auth, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack 인증 실패: %w", err)
	}
	return &Client{
		api:    api,
		socket: socketmode.New(api),
		botID:  auth.UserID,
	}, nil
}

// Send 는 채널로 메시지를 전송한다.
func (c *Client) Send(ctx context.Context, reply domain.Reply) error {
	if _, _, err := c.api.PostMessageContext(ctx, reply.ChannelID, slackgo.MsgOptionText(reply.Text, false)); err != nil {
		return fmt.Errorf("slack 메시지 전송 실패: %w", err)
	}
	return nil
}

// Run 은 이벤트 루프를 실행한다(ctx 취소까지 블로킹).
func (c *Client) Run(ctx context.Context, handler Handler) error {
	go func() {
		for evt := range c.socket.Events {
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			c.socket.Ack(*evt.Request)
			c.dispatch(ctx, eventsAPI, handler)
		}
	}()
	if err := c.socket.RunContext(ctx); err != nil {
		return fmt.Errorf("socket 실행 종료: %w", err)
	}
	return nil
}

// dispatch 는 Slack 이벤트를 IncomingMessage 로 변환해 handler 에 전달한다.
// app_mention 과 DM(im) 만 처리하고, 봇 자신/서브타입 메시지는 무시한다.
func (c *Client) dispatch(ctx context.Context, event slackevents.EventsAPIEvent, handler Handler) {
	if event.Type != slackevents.CallbackEvent {
		return
	}

	var in domain.IncomingMessage
	switch ev := event.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
	case *slackevents.MessageEvent:
		if ev.ChannelType != "im" || ev.SubType != "" || ev.BotID != "" || ev.User == c.botID {
			return
		}
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
	default:
		return
	}

	if err := handler.Handle(ctx, in); err != nil {
		log.FromContext(ctx).Error("메시지 처리 실패", "error", err)
	}
}
```

- [ ] **Step 2: 의존성 받기 + 빌드 확인**

Run:
```bash
cd ~/personal-agent/jarvis
go get github.com/slack-go/slack
go build ./internal/slack/
```
Expected: 성공. (실패 시 slack-go API 시그니처를 설치된 버전 기준으로 확인)

- [ ] **Step 3: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add internal/slack/client.go go.mod go.sum
git commit -m "feat: Socket Mode 연결/이벤트 디스패치 클라이언트 추가

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: cmd/server/main.go — 조립

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: main 작성**

Create `~/personal-agent/jarvis/cmd/server/main.go`:
```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jongseong0111/jarvis/internal/slack"
	"github.com/Jongseong0111/jarvis/pkg/config"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

func main() {
	logger := log.New(os.Getenv("JARVIS_ENV"))

	cfg, err := config.New()
	if err != nil {
		logger.Error("설정 로드 실패", "error", err)
		os.Exit(1)
	}

	client, err := slack.NewClient(cfg.SlackBotToken, cfg.SlackAppToken)
	if err != nil {
		logger.Error("slack 클라이언트 생성 실패", "error", err)
		os.Exit(1)
	}
	handler := slack.NewHandler(client)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.WithContext(ctx, logger)

	logger.Info("jarvis 시작", "env", cfg.Env)
	if err := client.Run(ctx, handler); err != nil {
		logger.Error("실행 종료", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 전체 빌드 + tidy + 테스트**

Run:
```bash
cd ~/personal-agent/jarvis
go mod tidy
go build ./...
go test ./...
```
Expected: 빌드 성공, 테스트 PASS.

- [ ] **Step 3: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add cmd/ go.mod go.sum
git commit -m "feat: main 조립(config→logger→slack client→이벤트 루프)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: 🧑 USER ACTION — Slack 앱 생성 + 토큰

**Files:** (없음 — Slack 콘솔 작업)

- [ ] **Step 1: 앱 생성 + Socket Mode**

api.slack.com/apps → Create New App (from scratch) → 워크스페이스 선택.
좌측 **Socket Mode** → Enable.

- [ ] **Step 2: Bot 토큰 스코프 부여**

**OAuth & Permissions → Bot Token Scopes** 에 추가:
`app_mentions:read`, `chat:write`, `im:history`, `im:read`, `im:write`.

- [ ] **Step 3: 이벤트 구독**

**Event Subscriptions** → Enable → **Subscribe to bot events** 에 추가:
`app_mention`, `message.im`.

- [ ] **Step 4: 앱 설치 + 토큰 확보**

**Install App** 으로 워크스페이스 설치 → `xoxb-...`(Bot Token) 확보.
**Basic Information → App-Level Tokens** → `connections:write` 스코프로 토큰 생성 → `xapp-...` 확보.

- [ ] **Step 5: .env 작성**

Run:
```bash
cp ~/personal-agent/jarvis/config/.env.example ~/personal-agent/jarvis/config/.env
```
`config/.env` 의 `SLACK_BOT_TOKEN`(xoxb-), `SLACK_APP_TOKEN`(xapp-) 채우기.

---

## Task 12: 수동 검증 — Slack echo 동작 확인

**Files:** (없음)

- [ ] **Step 1: 서버 실행**

Run:
```bash
cd ~/personal-agent/jarvis && make run
```
Expected: `jarvis 시작` 로그 출력, 연결 유지(블로킹).

- [ ] **Step 2: 🧑 USER ACTION — Slack에서 테스트**

- 채널에 봇 초대 후 `@jarvis 안녕` 멘션 → 봇이 `안녕` 응답하는지 확인.
- 봇에게 DM `테스트` 전송 → 봇이 `테스트` 응답하는지 확인.

Expected: 두 경우 모두 동일 텍스트 echo. (실패 시 토큰/스코프/이벤트구독 재확인, 서버 로그 확인)

- [ ] **Step 3: 서버 종료**

`Ctrl+C` → graceful 종료 확인.

---

## Task 13: CLAUDE.md 갱신

**Files:**
- Modify: `~/personal-agent/jarvis/CLAUDE.md`

- [ ] **Step 1: "권장 디렉터리 구조" 섹션 교체**

`CLAUDE.md`의 `## 권장 디렉터리 구조` 코드블록을 실제 구조로 교체:
```txt
~/personal-agent/
  jarvis/                 # 이 에이전트 서버 (Go)
    cmd/server/main.go
    domain/               # 인터페이스/DTO (구현 없음)
    internal/slack/       # Slack 채널 어댑터
    pkg/{config,log}/
    config/.env
  knowledge-base/         # 별도 git repo (지식 저장소, Claude Code skills 기반)
```

- [ ] **Step 2: 컨벤션/지식저장소 현황 반영**

`CLAUDE.md`에 다음 내용 추가/수정:
- 컨벤션은 `acloset-api` Clean Architecture를 규모에 맞게 차용(로깅 slog, config .env). 상세는 `docs/superpowers/specs/2026-06-15-jarvis-phase1-design.md` §4 참조.
- 지식 저장소는 이미 Claude Code skills 기반으로 구현됨(`/kb-ingest`, `/kb-approve` 등). 위치 `~/personal-agent/knowledge-base`.
- 환경변수 예시의 `KNOWLEDGE_REPO_PATH` 를 `~/personal-agent/knowledge-base` 로 수정.

- [ ] **Step 3: 커밋**

Run:
```bash
cd ~/personal-agent/jarvis
git add CLAUDE.md
git commit -m "docs: CLAUDE.md 를 현재 구조/컨벤션 기준으로 갱신

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: 정리 — base/ 삭제 + (선택) push

**Files:**
- Delete: `~/acloset-agent/base/`

- [ ] **Step 1: base/ 가 비었는지 확인**

Run:
```bash
find ~/acloset-agent/base -type f -not -path '*/.idea/*' -not -path '*/.claude/*'
```
Expected: 출력 없음(모든 내용 이동 완료). 남은 파일 있으면 적절히 이동 후 진행.

- [ ] **Step 2: base/ 삭제**

Run:
```bash
rm -rf ~/acloset-agent/base
```

- [ ] **Step 3: 🧑 USER ACTION — push 승인**

원격(GitHub 개인계정)에 올릴지 사용자 확인. 승인 시:
```bash
git -C ~/personal-agent/jarvis push -u origin main
git -C ~/personal-agent/knowledge-base push -u origin main
```
(브랜치명이 `master`면 그에 맞게. CLAUDE.md 안전원칙: push는 사용자 승인 후.)

---

## Self-Review 결과

- **Spec 커버리지**: §2 구조재편→Task 2,3 / §3 git identity→Task 1 / §4 컨벤션→Task 4 + 코드 전반 / §5 Phase1 echo→Task 5~10 / §6 테스트→Task 5,6,8 + Task 12 / §7 CLAUDE.md→Task 13. 누락 없음.
- **Placeholder**: 모든 코드 단계에 실제 코드 포함. TBD/TODO 없음.
- **타입 일관성**: `IncomingMessage{ChannelID,UserID,Text}`, `Reply{ChannelID,Text}`, `MessageSender.Send(ctx,Reply)`, `NewHandler`/`Handle`/`buildEcho`, `NewClient`/`Send`/`Run`, `config.New`/`Config`/`validate`, `log.New`/`WithContext`/`FromContext` — Task 간 시그니처 일치 확인.
- **알려진 리스크**: slack-go/golangci-lint/mockery 의 설치 버전에 따라 API/설정 키가 다를 수 있음 → 해당 Task에서 빌드 실패 시 설치 버전 기준 조정.
