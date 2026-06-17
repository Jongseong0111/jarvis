# 사진 → 물건 판별 (집정리 비전 입력) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slack 멘션에 사진을 첨부하면 사진 속 물건을 자동 인식해 기존 add_items 변경안+승인 흐름으로 집정리에 등록한다.

**Architecture:** 비전은 flash-lite로 "사진에 뭐 있나" 물건 이름만 추출 → 그 목록을 사용자 텍스트 앞에 주입 → 기존 flash 에이전트 루프가 장소 resolve·카테고리·변경안·승인을 그대로 처리. 새 표면 최소, 기존 흐름 무변경.

**Tech Stack:** Go 1.25, `google.golang.org/genai` v1.60.0 (InlineData 이미지 파트 + JSON ResponseSchema), `github.com/slack-go/slack` v0.26.0 (`GetFileContext` 봇토큰 다운로드).

## Global Constraints

- 한국어 주석/커밋, value receiver, 생성자 주입(`New...` 반환 인터페이스), table-driven 테스트 `t.Parallel()`.
- 쓰기는 항상 승인 버튼 경유(LLM이 Notion 직접 수정 금지). 사진은 입력 보조일 뿐, 등록은 add_items 변경안 승인으로만.
- 모델명은 config 값: `GEMINI_MODEL`(flash, 기본 `gemini-2.5-flash`) = 에이전트 로직, `GEMINI_VISION_MODEL`(flash-lite, 기본 `gemini-2.5-flash-lite`) = 비전. 2.5 deprecation 대비 한 줄 교체 가능해야 함.
- 비전 호출/다운로드 실패는 전체 실패 금지 — 로그 후 텍스트만으로 진행(best-effort).
- 이미지 MIME(`image/*`)만 취급. 다운로드 상한 10MB.
- 모든 패키지 `go test ./...`, `go vet ./...`, `go build ./...` green 유지.

---

### Task 1: domain — IncomingMessage 에 이미지 필드 추가

**Files:**
- Modify: `domain/slack.go`

**Interfaces:**
- Produces: `domain.Image{Data []byte; MIME string}`, `IncomingMessage.Images []Image` — Task 3/4/5 가 사용.

- [ ] **Step 1: `domain/slack.go` 에 Image 타입 + Images 필드 추가**

`IncomingMessage` 구조체를 아래로 교체:

```go
// IncomingMessage 는 채널에서 수신한 사용자 메시지다.
type IncomingMessage struct {
	ChannelID string
	UserID    string
	Text      string
	Images    []Image // 첨부 이미지(없으면 nil)
}

// Image 는 첨부 이미지의 원본 바이트와 MIME 타입이다.
type Image struct {
	Data []byte
	MIME string
}
```

- [ ] **Step 2: 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go build ./...`
Expected: 성공(기존 코드가 새 필드를 안 쓰므로 무변경 컴파일)

- [ ] **Step 3: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add domain/slack.go
git commit -m "feat(domain): IncomingMessage 에 첨부 이미지 필드 추가"
```

---

### Task 2: config — GEMINI_VISION_MODEL 추가

**Files:**
- Modify: `pkg/config/config.go`
- Test: `pkg/config/config_test.go` (없으면 생성)

**Interfaces:**
- Produces: `Config.GeminiVisionModel string` (기본 `gemini-2.5-flash-lite`) — Task 6 이 사용.

- [ ] **Step 1: 실패 테스트 작성**

`pkg/config/config_test.go` 에 추가(파일 없으면 아래 전체로 생성):

```go
package config

import (
	"os"
	"testing"
)

func TestNew_visionModelDefault(t *testing.T) {
	// 필수 env 채우고 VISION 모델은 비워 기본값 확인
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	os.Unsetenv("GEMINI_VISION_MODEL")

	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GeminiVisionModel != "gemini-2.5-flash-lite" {
		t.Fatalf("기본 비전 모델 = %q, want gemini-2.5-flash-lite", cfg.GeminiVisionModel)
	}
}

func TestNew_visionModelOverride(t *testing.T) {
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
		"GEMINI_VISION_MODEL": "gemini-3.1-flash-lite",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GeminiVisionModel != "gemini-3.1-flash-lite" {
		t.Fatalf("오버라이드 = %q", cfg.GeminiVisionModel)
	}
}
```

> 참고: `New()` 는 `godotenv.Load("config/.env")` 를 호출하지만 테스트 작업디렉터리(`pkg/config`)엔 그 파일이 없어 무시된다. `t.Setenv` 가 우선한다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./pkg/config/ -run TestNew_visionModel -v`
Expected: FAIL (`cfg.GeminiVisionModel` 필드 없음 → 컴파일 에러)

- [ ] **Step 3: config 구현**

`pkg/config/config.go` 의 `Config` 구조체에 필드 추가(`GeminiModel` 아래):

```go
	GeminiModel       string
	GeminiVisionModel string
```

`New()` 의 cfg 초기화에 추가(`GeminiModel:` 줄 아래):

```go
		GeminiModel:       getenv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiVisionModel: getenv("GEMINI_VISION_MODEL", "gemini-2.5-flash-lite"),
```

> 주의: 기존 `GeminiModel` 기본값이 `gemini-2.5-flash-lite` 로 되어 있으나 실제 `.env` 는 `gemini-2.5-flash` 를 지정해 동작 중이다. 기본값도 실제 운용에 맞춰 `gemini-2.5-flash` 로 바꾼다(위 코드대로). `validate()` 는 변경하지 않는다(비전 모델은 기본값 있어 선택).

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./pkg/config/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add pkg/config/config.go pkg/config/config_test.go
git commit -m "feat(config): GEMINI_VISION_MODEL 추가(기본 flash-lite)"
```

---

### Task 3: gemini — ExtractItems 비전 메서드

**Files:**
- Create: `internal/gemini/vision.go`
- Test: `internal/gemini/vision_test.go`

**Interfaces:**
- Consumes: `domain.Image` (Task 1), 기존 `Client{apiKey, model}`, `requestTimeout`.
- Produces: `func (c *Client) ExtractItems(ctx context.Context, images []domain.Image) ([]string, error)` — Task 4 의 `VisionExtractor` 를 만족. 순수 헬퍼 `dedupeNames([]string) []string`.

> 네트워크 호출부(genai → Google)는 기존 `GenerateWithTools` 와 동일하게 **라이브 검증**한다(httptest 불가). TDD 는 순수 헬퍼 `dedupeNames` 로 한다.

- [ ] **Step 1: 실패 테스트 작성**

`internal/gemini/vision_test.go`:

```go
package gemini

import (
	"reflect"
	"testing"
)

func TestDedupeNames(t *testing.T) {
	t.Parallel()
	got := dedupeNames([]string{"휴지", "휴지", "물티슈", "", "  휴지  ", "정리함"})
	want := []string{"휴지", "물티슈", "정리함"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeNames = %v, want %v", got, want)
	}
}

func TestDedupeNames_empty(t *testing.T) {
	t.Parallel()
	if got := dedupeNames([]string{"", "   "}); len(got) != 0 {
		t.Fatalf("빈 입력 = %v, want []", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/gemini/ -run TestDedupeNames -v`
Expected: FAIL (`dedupeNames` 미정의)

- [ ] **Step 3: vision.go 구현**

`internal/gemini/vision.go` 생성:

```go
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

// visionPrompt 는 사진에서 정리/수납 대상 물건 이름만 뽑게 지시한다.
const visionPrompt = `이 사진들에 보이는, 옮기거나 수납할 수 있는 물건들을 한국어 이름의 JSON 배열로만 반환해라.
가구·벽·바닥·문·창문 같은 배경 구조물은 제외하고, 정리하거나 수납할 수 있는 물건만 포함해라.
같은 물건이 여러 개여도 이름은 한 번만. 확실하지 않으면 제외한다.`

// ExtractItems 는 이미지들에서 물건 이름 목록을 추출한다(비전 모델 사용).
func (c *Client) ExtractItems(ctx context.Context, images []domain.Image) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	parts := []*genai.Part{{Text: visionPrompt}}
	for _, img := range images {
		parts = append(parts, &genai.Part{InlineData: &genai.Blob{Data: img.Data, MIMEType: img.MIME}})
	}
	contents := []*genai.Content{{Role: genai.RoleUser, Parts: parts}}

	temp := float32(0)
	thinkBudget := int32(0) // thinking 비활성(속도/비용)
	cfg := &genai.GenerateContentConfig{
		Temperature:      &temp,
		ThinkingConfig:   &genai.ThinkingConfig{ThinkingBudget: &thinkBudget},
		ResponseMIMEType: "application/json",
		ResponseSchema:   &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	}

	resp, err := client.Models.GenerateContent(ctx, c.model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini 비전 생성 실패: %w", err)
	}

	var names []string
	if err := json.Unmarshal([]byte(resp.Text()), &names); err != nil {
		return nil, fmt.Errorf("비전 응답 파싱 실패: %w (raw=%q)", err, resp.Text())
	}
	return dedupeNames(names), nil
}

// dedupeNames 는 공백 제거 후 빈 값/중복을 걸러 순서를 보존한 목록을 만든다.
func dedupeNames(names []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
```

- [ ] **Step 4: 테스트 통과 + 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/gemini/ -v && go build ./...`
Expected: PASS, 빌드 성공

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/gemini/vision.go internal/gemini/vision_test.go
git commit -m "feat(gemini): 사진에서 물건 이름 추출하는 ExtractItems 추가"
```

---

### Task 4: agent — VisionExtractor 주입 + Route 비전 전처리

**Files:**
- Modify: `internal/agent/agent.go`
- Test: `internal/agent/agent_test.go`

**Interfaces:**
- Consumes: `domain.Image` (Task 1), `(*gemini.Client).ExtractItems` (Task 3) 를 만족하는 인터페이스.
- Produces: `VisionExtractor` 인터페이스, `New(gen generator, vision VisionExtractor, tools []Tool, system string) Agent` (시그니처 변경 — vision 추가).

- [ ] **Step 1: 실패 테스트 작성 — fakeGen 에 contents 캡처 추가 + fakeVision + 3 케이스**

`internal/agent/agent_test.go` 의 `fakeGen` 구조체와 메서드를 아래로 교체(필드/대입 추가):

```go
type fakeGen struct {
	responses    []*genai.GenerateContentResponse
	i            int
	calls        int
	lastContents []*genai.Content
}

func (f *fakeGen) GenerateWithTools(_ context.Context, contents []*genai.Content, _ []*genai.Tool, _ string) (*genai.GenerateContentResponse, error) {
	f.calls++
	f.lastContents = contents
	r := f.responses[f.i%len(f.responses)]
	f.i++
	return r, nil
}
```

`newAgent` 헬퍼를 vision 인자에 맞춰 교체:

```go
func newAgent(gen generator, port HomePort) Agent {
	return New(gen, nil, HomeTools(port, "", ""), "")
}
```

같은 파일에 fakeVision + 테스트 3개 추가:

```go
type fakeVision struct {
	names []string
	err   error
	calls int
}

func (f *fakeVision) ExtractItems(context.Context, []domain.Image) ([]string, error) {
	f.calls++
	return f.names, f.err
}

func TestAgent_vision_augmentsText(t *testing.T) {
	t.Parallel()
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{textResp("등록할게")}}
	vis := &fakeVision{names: []string{"정리함", "휴지"}}
	a := New(gen, vis, HomeTools(&fakeHomePort{}, "", ""), "")

	_, err := a.Route(context.Background(), domain.IncomingMessage{
		ChannelID: "C1", Text: "안방 수납장1에 넣었어",
		Images: []domain.Image{{Data: []byte("x"), MIME: "image/jpeg"}},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if vis.calls != 1 {
		t.Fatalf("vision 호출 수 = %d, want 1", vis.calls)
	}
	last := gen.lastContents[len(gen.lastContents)-1]
	if !strings.Contains(last.Parts[0].Text, "정리함") || !strings.Contains(last.Parts[0].Text, "안방 수납장1") {
		t.Fatalf("증강 텍스트 = %q", last.Parts[0].Text)
	}
}

func TestAgent_vision_emptyNoText_asksBack(t *testing.T) {
	t.Parallel()
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{textResp("안 불려야 함")}}
	vis := &fakeVision{names: []string{}}
	a := New(gen, vis, HomeTools(&fakeHomePort{}, "", ""), "")

	reply, err := a.Route(context.Background(), domain.IncomingMessage{
		ChannelID: "C1", Text: "",
		Images: []domain.Image{{Data: []byte("x"), MIME: "image/jpeg"}},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if gen.calls != 0 {
		t.Fatalf("빈 인식+무텍스트면 에이전트 루프 안 돌아야 함: calls=%d", gen.calls)
	}
	if !strings.Contains(reply.Text, "못 찾") {
		t.Fatalf("되묻기 응답 = %q", reply.Text)
	}
}

func TestAgent_vision_errorFallsBackToText(t *testing.T) {
	t.Parallel()
	gen := &fakeGen{responses: []*genai.GenerateContentResponse{textResp("그냥 답")}}
	vis := &fakeVision{err: errInjected}
	a := New(gen, vis, HomeTools(&fakeHomePort{}, "", ""), "")

	reply, err := a.Route(context.Background(), domain.IncomingMessage{
		ChannelID: "C1", Text: "안녕",
		Images: []domain.Image{{Data: []byte("x"), MIME: "image/jpeg"}},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if gen.calls != 1 || reply.Text != "그냥 답" {
		t.Fatalf("비전 실패 시 텍스트로 진행해야 함: calls=%d reply=%q", gen.calls, reply.Text)
	}
}
```

테스트 파일 import 에 `"errors"` 추가하고, 파일 상단(import 블록 아래)에 sentinel 추가:

```go
var errInjected = errors.New("injected")
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/agent/ -run TestAgent_vision -v`
Expected: FAIL (`New` 가 4번째 인자/vision 없음 → 컴파일 에러)

- [ ] **Step 3: agent.go 구현 — VisionExtractor 인터페이스 + 필드 + New + Route 전처리**

`internal/agent/agent.go` 의 `generator` 인터페이스 아래에 추가:

```go
// VisionExtractor 는 이미지에서 물건 이름 목록을 뽑는 능력이다(테스트에서 fake 주입).
type VisionExtractor interface {
	ExtractItems(ctx context.Context, images []domain.Image) ([]string, error)
}
```

`Agent` 구조체에 `vision` 필드 추가:

```go
type Agent struct {
	gen    generator
	vision VisionExtractor
	tools  map[string]Tool
	decls  []*genai.Tool
	system string
	mem    *memory
}
```

`New` 시그니처/본문 교체:

```go
// New 는 Agent 를 생성한다. vision 은 nil 가능(이미지 입력 미사용 시).
func New(gen generator, vision VisionExtractor, tools []Tool, system string) Agent {
	if system == "" {
		system = DefaultSystemPrompt
	}
	return Agent{gen: gen, vision: vision, tools: toolMap(tools), decls: toolDecls(tools), system: system, mem: newMemory()}
}
```

`Route` 본문 맨 앞(`contents := ...` 줄 위)에 비전 전처리 삽입:

```go
func (a Agent) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	if len(in.Images) > 0 && a.vision != nil {
		names, err := a.vision.ExtractItems(ctx, in.Images)
		switch {
		case err != nil:
			log.FromContext(ctx).Error("비전 추출 실패", "error", err) // best-effort: 텍스트로 진행
		case len(names) == 0:
			if strings.TrimSpace(in.Text) == "" {
				return domain.Reply{ChannelID: in.ChannelID, Text: "사진에서 물건을 못 찾았어. 뭐가 있는지 말로 알려줄래?"}, nil
			}
		default:
			in.Text = "[사진에서 인식한 물건: " + strings.Join(names, ", ") + "] " + in.Text
		}
	}

	contents := append(a.mem.get(in.ChannelID), genai.Text(in.Text)...)
	// ... 이하 기존 동일
```

`agent.go` 의 import 블록에 로거 추가(기존엔 없음 — `internal/slack/handler.go` 와 동일 경로):

```go
	"github.com/Jongseong0111/jarvis/pkg/log"
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/agent/ -v`
Expected: PASS (기존 테스트 + 신규 vision 테스트 모두)

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat(agent): 이미지 입력 시 비전으로 물건 인식해 텍스트 증강"
```

---

### Task 5: slack — 첨부 이미지 다운로드 + 빈 텍스트 가드

**Files:**
- Modify: `internal/slack/client.go`
- Modify: `internal/slack/handler.go`
- Test: `internal/slack/client_test.go` (없으면 생성)

**Interfaces:**
- Consumes: `domain.Image` (Task 1), `slackgo.File`(`Mimetype`, `URLPrivateDownload`/`URLPrivate`), `(*slackgo.Client).GetFileContext(ctx, url, io.Writer)`.
- Produces: `IncomingMessage.Images` 채워서 핸들러로 전달. 순수 헬퍼 `isImageMime(string) bool`, 최대 크기 상수 `maxImageBytes`.

- [ ] **Step 1: 실패 테스트 작성 — isImageMime 순수 헬퍼**

`internal/slack/client_test.go`:

```go
package slack

import "testing"

func TestIsImageMime(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/webp":      true,
		"application/pdf": false,
		"text/plain":      false,
		"":                false,
	}
	for mime, want := range cases {
		if got := isImageMime(mime); got != want {
			t.Errorf("isImageMime(%q) = %v, want %v", mime, got, want)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/slack/ -run TestIsImageMime -v`
Expected: FAIL (`isImageMime` 미정의)

- [ ] **Step 3: client.go 구현 — 헬퍼 + 다운로드 + dispatch 연결**

`internal/slack/client.go` import 에 추가:

```go
	"bytes"
	"strings"
```

파일 끝에 헬퍼 추가:

```go
// maxImageBytes 는 다운로드 허용 이미지 최대 크기다(과대 파일 방지).
const maxImageBytes = 10 * 1024 * 1024

// isImageMime 은 image/* MIME 인지 판별한다.
func isImageMime(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

// downloadImages 는 Slack 첨부 중 이미지들을 봇토큰으로 다운로드한다.
// 개별 실패는 건너뛴다(best-effort) — 전체를 실패시키지 않는다.
func (c *Client) downloadImages(ctx context.Context, files []slackgo.File) []domain.Image {
	var out []domain.Image
	for _, f := range files {
		if !isImageMime(f.Mimetype) {
			continue
		}
		url := f.URLPrivateDownload
		if url == "" {
			url = f.URLPrivate
		}
		if url == "" {
			continue
		}
		var buf bytes.Buffer
		if err := c.api.GetFileContext(ctx, url, &buf); err != nil {
			log.FromContext(ctx).Error("이미지 다운로드 실패", "error", err, "url", url)
			continue
		}
		if buf.Len() == 0 || buf.Len() > maxImageBytes {
			log.FromContext(ctx).Error("이미지 크기 부적합", "bytes", buf.Len())
			continue
		}
		out = append(out, domain.Image{Data: buf.Bytes(), MIME: f.Mimetype})
	}
	return out
}
```

`dispatch` 의 두 case 에서 Files 를 다운로드해 Images 에 채운다. `AppMentionEvent` case:

```go
	case *slackevents.AppMentionEvent:
		// 다른 봇이 @jarvis 를 멘션하는 경우 무시한다.
		if ev.BotID != "" {
			return
		}
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
		in.Images = c.downloadImages(ctx, ev.Files)
```

`MessageEvent` case:

```go
	case *slackevents.MessageEvent:
		if ev.ChannelType != "im" || ev.SubType != "" || ev.BotID != "" || ev.User == c.botID {
			return
		}
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
		in.Images = c.downloadImages(ctx, ev.Files)
```

> 주의: `ev.SubType != ""` 가드는 유지한다. 이미지 첨부 메시지의 SubType 은 비어 있다(`file_share` SubType 은 봇 업로드 등 특수 케이스에만 붙음 — 일반 사용자 첨부는 빈 SubType). 그대로 둔다.

- [ ] **Step 4: handler.go — 빈 텍스트 가드를 이미지까지 고려하도록 수정**

`internal/slack/handler.go` 의 `Handle` 에서:

```go
	in.Text = cleanText(in.Text)
	if in.Text == "" {
		return nil
	}
```

를 아래로 교체(이미지만 있어도 진행):

```go
	in.Text = cleanText(in.Text)
	if in.Text == "" && len(in.Images) == 0 {
		return nil
	}
```

- [ ] **Step 5: 테스트 통과 + 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/slack/ -v && go build ./...`
Expected: PASS, 빌드 성공

- [ ] **Step 6: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/slack/client.go internal/slack/handler.go internal/slack/client_test.go
git commit -m "feat(slack): 첨부 이미지 봇토큰 다운로드 + 이미지-only 메시지 허용"
```

---

### Task 6: 조립 + 전체 검증 + 라이브 체크리스트

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `config/.env` (선택, 주석 한 줄)

**Interfaces:**
- Consumes: `cfg.GeminiVisionModel` (Task 2), `gemini.New` (기존), `agent.New(gen, vision, tools, system)` (Task 4).

- [ ] **Step 1: main.go 조립 — 비전 클라이언트 생성 후 agent 에 주입**

`cmd/server/main.go` 에서 `geminiClient := ...` 줄 아래에 비전 클라이언트 추가:

```go
	geminiClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel)
	visionClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiVisionModel)
```

`ag := agent.New(...)` 호출을 vision 인자 포함으로 교체:

```go
	ag := agent.New(geminiClient, visionClient, agent.HomeTools(home, cfg.NotionHomeURL, mapURL), "")
```

- [ ] **Step 2: 전체 빌드/vet/테스트**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go build ./... && go vet ./... && go test ./...`
Expected: 모두 성공, 모든 패키지 ok

- [ ] **Step 3: (선택) .env 에 비전 모델 명시**

`config/.env` 에 한 줄 추가(없어도 기본값 동작, 명시로 가독성):

```
GEMINI_VISION_MODEL=gemini-2.5-flash-lite
```

- [ ] **Step 4: 서버 재시작 (기존 운영 방식)**

```bash
cd /Users/seonghyun/personal-agent/jarvis
pkill -f bin/jarvis 2>/dev/null
go build -o bin/jarvis ./cmd/server
nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
sleep 2 && tail -5 /tmp/jarvis.log
```

Expected: 로그에 "jarvis 시작"

- [ ] **Step 5: 라이브 검증 (Slack, 수동 — 사용자와 함께)**

다음을 Slack 에서 확인:
1. 멘션 `@jarvis 안방 수납장1에 넣었어` + 물건 여러 개 찍힌 사진 → "정리함/휴지/... 인식" → add_items 변경안 + 버튼 → 승인 → Notion 생성 + 지도 갱신.
2. 사진만(텍스트 없이, 멘션만) + 사진 → 물건 인식해서 보여주고 "어디에 넣은 거야?" 되묻기.
3. 물건 안 보이는 사진(벽/풍경) → "사진에서 물건을 못 찾았어".
4. 이미지 없는 일반 텍스트 → 기존과 동일(회귀 없음).

- [ ] **Step 6: 커밋 (라이브 검증 후)**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add cmd/server/main.go config/.env
git commit -m "feat: 사진→물건 판별 조립(비전 클라이언트 주입) + 라이브 검증"
```

> `config/.env` 는 gitignore 라 실제로 스테이징되지 않는다(커밋엔 main.go 만 포함). 정상.

---

## 라이브 검증 전 사용자 확인 필요

- **Slack 앱 권한**: 봇이 파일을 읽으려면 `files:read` OAuth scope 가 필요하다. 없으면 `GetFileContext` 가 401/403. Slack 앱 설정에서 `files:read` 추가 후 워크스페이스 재설치 필요할 수 있음 — 라이브 검증 시 실패하면 이 scope 부터 확인.
- 비전 호출은 회사 Gemini 계정 과금(메시지당 ~0.1센트 수준, 사진 토큰 무시 가능).
