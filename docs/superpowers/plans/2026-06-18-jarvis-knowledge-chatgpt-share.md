# 지식저장소 ChatGPT 공유링크 요약 (Phase A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slack에 ChatGPT 공유 링크를 보내면 jarvis가 대화를 추출·요약해 보여주고, 대화로 수정한 뒤 "저장해"하면 knowledge-base 레포 `sources/`에 기록한다.

**Architecture:** 기존 tool-calling 에이전트에 도구 2개 추가 — `summarize_chatgpt_share`(읽기: curl 추출 + Gemini 요약, 저장 안 함)와 `save_kb_source`(저장: 에이전트가 넘긴 최종 요약을 파일로). 수정은 순수 대화(에이전트가 맥락에서 요약을 고침). 새 `internal/knowledge` 패키지가 추출·저장을 담당.

**Tech Stack:** Go 1.25, stdlib `net/http`(공유 페이지 fetch)·`regexp`(임베드 대화 추출)·`os`/`filepath`(파일), 기존 `internal/gemini`(요약), 기존 `internal/agent` Tool 추상화.

## Global Constraints

- 한국어 주석/커밋, value receiver, 생성자 주입(`New...`), table-driven 테스트 `t.Parallel()`.
- **네트워크 호출(Gemini, HTTP fetch)은 unit test 안 함 — 라이브 검증**(기존 `GenerateWithTools` 컨벤션). 순수 로직(파싱·슬러그·파일쓰기)만 TDD.
- 지식 도구는 **읽기형 도구**(`Tool.Run`)만 사용 — 변경안/버튼 없음. 저장은 사용자의 명시적 "저장해"가 승인. (Phase A는 미커밋 로컬 파일만 건드림 — 외부/파괴적 변경 없음.)
- `KNOWLEDGE_REPO_PATH` 기본값 `~/personal-agent/knowledge-base`(`~` 확장).
- 소스 경로: `sources/conversation/<YYYY-MM-DD>-<slug>.md`. 중복 시 `-2`,`-3` 접미.
- 모든 패키지 `go build ./...`·`go vet ./...`·`go test ./...` green.
- 이 브랜치는 main 기준(사진 기능 없음). `agent.New(gen, tools, system)` 3-arg.

---

### Task 1: config — KNOWLEDGE_REPO_PATH

**Files:**
- Modify: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `Config.KnowledgeRepoPath string`(기본 `~/personal-agent/knowledge-base`, `~` 확장). Task 7이 사용.

- [ ] **Step 1: 실패 테스트 작성**

`pkg/config/config_test.go` 에 추가(파일 없으면 아래 전체로 생성):

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	env := map[string]string{
		"SLACK_BOT_TOKEN": "x", "SLACK_APP_TOKEN": "x", "GEMINI_API_KEY": "x",
		"NOTION_API_KEY": "x", "NOTION_LOCATIONS_DB_ID": "x",
		"NOTION_CATEGORIES_DB_ID": "x", "NOTION_ITEMS_DB_ID": "x",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestNew_knowledgeRepoPathDefault(t *testing.T) {
	setRequiredEnv(t)
	os.Unsetenv("KNOWLEDGE_REPO_PATH")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "personal-agent", "knowledge-base")
	if cfg.KnowledgeRepoPath != want {
		t.Fatalf("기본 경로 = %q, want %q", cfg.KnowledgeRepoPath, want)
	}
}

func TestNew_knowledgeRepoPathOverrideExpandsTilde(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KNOWLEDGE_REPO_PATH", "~/kb")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.KnowledgeRepoPath != filepath.Join(home, "kb") {
		t.Fatalf("~ 확장 실패: %q", cfg.KnowledgeRepoPath)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./pkg/config/ -run TestNew_knowledgeRepoPath -v`
Expected: FAIL (`KnowledgeRepoPath` 필드 없음 → 컴파일 에러)

- [ ] **Step 3: 구현**

`pkg/config/config.go` import 에 `path/filepath` 추가. `Config` 구조체에 필드 추가(`GeminiModel` 아래):

```go
	GeminiModel       string
	KnowledgeRepoPath string
```

`New()` cfg 초기화에 추가(`GeminiModel:` 줄 아래):

```go
		KnowledgeRepoPath: expandHome(getenv("KNOWLEDGE_REPO_PATH", "~/personal-agent/knowledge-base")),
```

파일 끝에 헬퍼 추가:

```go
// expandHome 은 "~/" 접두를 사용자 홈 디렉터리로 치환한다.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
```

`pkg/config/config.go` import 에 `strings` 도 추가(없으면).

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./pkg/config/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add pkg/config/config.go pkg/config/config_test.go
git commit -m "feat(config): KNOWLEDGE_REPO_PATH 추가(~ 확장)"
```

---

### Task 2: gemini — GenerateText (도구 없는 일반 생성)

**Files:**
- Create: `internal/gemini/generate.go`

**Interfaces:**
- Consumes: 기존 `Client{apiKey, model}`, `requestTimeout`(client.go).
- Produces: `func (c *Client) GenerateText(ctx context.Context, system, user string) (string, error)`. Task 5의 `summarizer` 인터페이스를 만족.

> 네트워크 호출이라 unit test 없음(기존 `GenerateWithTools` 와 동일). `go build` 로만 검증.

- [ ] **Step 1: 구현**

`internal/gemini/generate.go` 생성:

```go
package gemini

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// GenerateText 는 도구 없이 일반 텍스트를 생성한다(요약 등).
func (c *Client) GenerateText(ctx context.Context, system, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	temp := float32(0)
	thinkBudget := int32(0)
	cfg := &genai.GenerateContentConfig{
		Temperature:    &temp,
		ThinkingConfig: &genai.ThinkingConfig{ThinkingBudget: &thinkBudget},
	}
	if system != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}

	resp, err := client.Models.GenerateContent(ctx, c.model, genai.Text(user), cfg)
	if err != nil {
		return "", fmt.Errorf("gemini 생성 실패: %w", err)
	}
	out := strings.TrimSpace(resp.Text())
	if out == "" {
		return "", fmt.Errorf("gemini 빈 응답")
	}
	return out, nil
}
```

- [ ] **Step 2: 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go build ./... && go vet ./internal/gemini/`
Expected: 성공

- [ ] **Step 3: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/gemini/generate.go
git commit -m "feat(gemini): 도구 없는 일반 텍스트 생성 GenerateText 추가"
```

---

### Task 3: knowledge — 공유 페이지 추출 (share.go)

**Files:**
- Create: `internal/knowledge/share.go`
- Create: `internal/knowledge/testdata/share-sample.html`
- Test: `internal/knowledge/share_test.go`

**Interfaces:**
- Produces: `Conversation{Title, URL string; Messages []string}`; `FetchConversation(ctx, url string) (Conversation, error)`(네트워크); 순수 `parseConversation(html []byte, url string) (Conversation, error)`. Task 5가 `FetchConversation` 사용.

> `FetchConversation` 의 HTTP 부분은 라이브 검증. 순수 `parseConversation` 만 픽스처로 TDD.

- [ ] **Step 1: 픽스처 작성**

`internal/knowledge/testdata/share-sample.html` 생성(실제 ChatGPT 공유 페이지의 임베드 형식을 축약 모방 — 메시지는 `\"...\"` 이스케이프 따옴표로 감싸이고, feature flag 같은 식별자 잡음이 섞여 있음):

```html
<!DOCTYPE html><html><head><title>ChatGPT - 고랭 장점 설명</title>
<meta property="og:title" content="이 채팅을 확인해 보세요"/></head>
<body>
<script type="application/json" id="bootstrap">{"flags":["update_custom_instructions_beacon_experiment_enabled","contextual_ia_stream_impl"]}</script>
<script>window.__reactRouterContext="...,\"linear_conversation\",\"고루틴은 경량 동시성 단위야. 너무 많이 띄우면 과부하될 수 있으니 수를 제한해야 해.\",\"update_custom_instructions_beacon_experiment_enabled\",\"채널은 고루틴끼리 데이터를 안전하게 주고받는 통로야. 공유 메모리 대신 채널로 흐름을 관리해 경쟁 상태를 피하지.\",\"contextual_ia_stream_impl\",\"워커 풀은 한정된 수의 고루틴이 채널로 작업을 받아 처리하는 패턴이야.\",...";</script>
</body></html>
```

- [ ] **Step 2: 실패 테스트 작성**

`internal/knowledge/share_test.go`:

```go
package knowledge

import (
	"os"
	"strings"
	"testing"
)

func TestParseConversation_extractsTitleAndMessages(t *testing.T) {
	t.Parallel()
	html, err := os.ReadFile("testdata/share-sample.html")
	if err != nil {
		t.Fatalf("픽스처 읽기: %v", err)
	}
	conv, err := parseConversation(html, "https://chatgpt.com/share/abc")
	if err != nil {
		t.Fatalf("parseConversation: %v", err)
	}
	if conv.Title != "고랭 장점 설명" {
		t.Fatalf("제목 = %q, want 고랭 장점 설명", conv.Title)
	}
	joined := strings.Join(conv.Messages, "\n")
	if !strings.Contains(joined, "고루틴") || !strings.Contains(joined, "워커 풀") {
		t.Fatalf("핵심 메시지 누락: %q", joined)
	}
	for _, m := range conv.Messages {
		if strings.Contains(m, "experiment_enabled") || strings.Contains(m, "stream_impl") {
			t.Fatalf("식별자 잡음이 메시지로 추출됨: %q", m)
		}
	}
}

func TestParseConversation_emptyErrors(t *testing.T) {
	t.Parallel()
	if _, err := parseConversation([]byte("<title>빈 페이지</title><body>no data</body>"), "u"); err == nil {
		t.Fatal("대화 없는 페이지는 에러여야 함")
	}
}

func TestLooksLikeMessage(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"고루틴은 경량 동시성 단위야. 수를 제한해.":                true,  // 한글 + 길이
		"worker pool limits concurrent goroutines safely": true,  // 영문 문장(공백)
		"update_custom_instructions_beacon_enabled":        false, // 식별자(공백·한글 없음)
		"short":                                            false, // 너무 짧음
		"https://example.com/some/long/path/here":          false, // URL
	}
	for in, want := range cases {
		if got := looksLikeMessage(in); got != want {
			t.Errorf("looksLikeMessage(%q) = %v, want %v", in, got, want)
		}
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -run 'TestParse|TestLooks' -v`
Expected: FAIL (`parseConversation`/`looksLikeMessage` 미정의)

- [ ] **Step 4: 구현**

`internal/knowledge/share.go` 생성:

```go
// Package knowledge 는 ChatGPT 공유 대화를 추출·요약·저장한다.
package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"

// Conversation 은 공유 페이지에서 추출한 대화다(화자 구분 없는 거친 메시지 목록).
type Conversation struct {
	Title    string
	URL      string
	Messages []string
}

var (
	sharePat = regexp.MustCompile(`^https?://(chatgpt\.com|chat\.openai\.com)/share/`)
	titleRe  = regexp.MustCompile(`<title>([^<]*)</title>`)
	// 임베드 스트림에서 이스케이프 따옴표(\") 로 감싼 문자열. 내부에 따옴표/역슬래시 없는 구간만.
	msgRe = regexp.MustCompile(`\\"([^"\\]{15,4000})\\"`)
)

// FetchConversation 은 공유 링크를 받아 대화를 추출한다(네트워크).
func FetchConversation(ctx context.Context, url string) (Conversation, error) {
	if !sharePat.MatchString(url) {
		return Conversation{}, fmt.Errorf("ChatGPT 공유 링크가 아니야: %s", url)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Conversation{}, err
	}
	req.Header.Set("User-Agent", browserUA)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Conversation{}, fmt.Errorf("공유 페이지 fetch 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Conversation{}, fmt.Errorf("공유 페이지 응답 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4MB
	if err != nil {
		return Conversation{}, err
	}
	return parseConversation(body, url)
}

// parseConversation 은 HTML 에서 제목과 대화 메시지를 거칠게 추출한다(순수 함수).
func parseConversation(html []byte, url string) (Conversation, error) {
	title := ""
	if m := titleRe.FindSubmatch(html); m != nil {
		title = strings.TrimSpace(string(m[1]))
		title = strings.TrimPrefix(title, "ChatGPT - ")
		title = strings.TrimSpace(strings.TrimPrefix(title, "ChatGPT"))
	}

	var msgs []string
	seen := map[string]bool{}
	total := 0
	for _, m := range msgRe.FindAllSubmatch(html, -1) {
		s := strings.TrimSpace(string(m[1]))
		if !looksLikeMessage(s) || seen[s] {
			continue
		}
		seen[s] = true
		msgs = append(msgs, s)
		total += len(s)
	}
	if len(msgs) == 0 || total < 100 {
		return Conversation{}, fmt.Errorf("대화를 추출하지 못했어")
	}
	return Conversation{Title: title, URL: url, Messages: msgs}, nil
}

// looksLikeMessage 는 추출 문자열이 (식별자/플래그가 아닌) 자연어 메시지인지 판별한다.
func looksLikeMessage(s string) bool {
	if len(s) < 15 || strings.HasPrefix(s, "http") {
		return false
	}
	if strings.ContainsRune(s, ' ') {
		return true
	}
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 { // 한글 음절
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: 테스트 통과 + 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -v && go build ./...`
Expected: PASS, 빌드 성공

- [ ] **Step 6: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/knowledge/share.go internal/knowledge/share_test.go internal/knowledge/testdata/share-sample.html
git commit -m "feat(knowledge): ChatGPT 공유 페이지에서 대화 추출(FetchConversation)"
```

---

### Task 4: knowledge — 소스 노트 저장 (source.go)

**Files:**
- Create: `internal/knowledge/source.go`
- Test: `internal/knowledge/source_test.go`

**Interfaces:**
- Produces: `WriteSource(repoPath, today, title, url, content string) (string, error)`; 순수 `slugify(string) string`. Task 5가 `WriteSource` 사용.

- [ ] **Step 1: 실패 테스트 작성**

`internal/knowledge/source_test.go`:

```go
package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"고랭 장점 설명":     "고랭-장점-설명",
		"Hello, World!":  "hello-world",
		"  공백  많은   제목 ": "공백-많은-제목",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSource_writesFrontmatterAndBody(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	path, err := WriteSource(repo, "2026-06-18", "고랭 장점 설명", "https://chatgpt.com/share/abc", "## 핵심\n- 고루틴")
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	want := filepath.Join(repo, "sources", "conversation", "2026-06-18-고랭-장점-설명.md")
	if path != want {
		t.Fatalf("경로 = %q, want %q", path, want)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	for _, sub := range []string{"title: 고랭 장점 설명", "source: chatgpt-share", "url: https://chatgpt.com/share/abc", "captured: 2026-06-18", "type: conversation", "## 핵심", "- 고루틴"} {
		if !strings.Contains(s, sub) {
			t.Fatalf("본문에 %q 없음:\n%s", sub, s)
		}
	}
}

func TestWriteSource_duplicateGetsSuffix(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	_, _ = WriteSource(repo, "2026-06-18", "중복", "", "a")
	path2, err := WriteSource(repo, "2026-06-18", "중복", "", "b")
	if err != nil {
		t.Fatalf("WriteSource 2: %v", err)
	}
	if !strings.HasSuffix(path2, "2026-06-18-중복-2.md") {
		t.Fatalf("중복 접미 실패: %q", path2)
	}
}

func TestWriteSource_omitsEmptyURL(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	path, _ := WriteSource(repo, "2026-06-18", "노URL", "", "본문")
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "url:") {
		t.Fatalf("빈 URL 은 frontmatter 에서 빠져야 함:\n%s", b)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -run 'TestSlugify|TestWriteSource' -v`
Expected: FAIL (`slugify`/`WriteSource` 미정의)

- [ ] **Step 3: 구현**

`internal/knowledge/source.go` 생성:

```go
package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// WriteSource 는 요약을 sources/conversation/<today>-<slug>.md 로 저장한다(미커밋).
// url 이 비면 frontmatter 에서 생략한다. 같은 경로가 있으면 -2,-3 접미를 붙인다.
func WriteSource(repoPath, today, title, url, content string) (string, error) {
	dir := filepath.Join(repoPath, "sources", "conversation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("디렉터리 생성 실패: %w", err)
	}

	slug := slugify(title)
	if slug == "" {
		slug = "untitled"
	}
	base := today + "-" + slug
	path := filepath.Join(dir, base+".md")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.md", base, i))
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + title + "\n")
	b.WriteString("source: chatgpt-share\n")
	if url != "" {
		b.WriteString("url: " + url + "\n")
	}
	b.WriteString("captured: " + today + "\n")
	b.WriteString("type: conversation\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("파일 쓰기 실패: %w", err)
	}
	return path, nil
}

// slugify 는 제목을 파일명용 슬러그로 바꾼다(한글 유지, 영문 소문자, 비단어→'-', 60룬 컷).
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if r := []rune(out); len(r) > 60 {
		out = strings.Trim(string(r[:60]), "-")
	}
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -v`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/knowledge/source.go internal/knowledge/source_test.go
git commit -m "feat(knowledge): 요약을 sources/ 에 저장하는 WriteSource 추가"
```

---

### Task 5: knowledge — Service (요약+저장 조립)

**Files:**
- Create: `internal/knowledge/service.go`
- Test: `internal/knowledge/service_test.go`

**Interfaces:**
- Consumes: `FetchConversation`(Task 3), `WriteSource`(Task 4), `summarizer` 인터페이스(`GenerateText` — `*gemini.Client`가 만족, Task 2).
- Produces: `Service` + `NewService(sum summarizer, repoPath string) Service`. 메서드 `Summarize(ctx, url) (title, summary string, err error)`(네트워크), `SaveSource(ctx, title, url, content string) (path string, err error)`. Task 6의 `agent.KnowledgePort`를 구조적으로 만족.

> `Summarize` 는 네트워크라 라이브 검증. `SaveSource`(파일)만 TDD.

- [ ] **Step 1: 실패 테스트 작성**

`internal/knowledge/service_test.go`:

```go
package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestService_SaveSource_writesFile(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svc := NewService(nil, repo) // 저장 경로는 summarizer 안 씀
	path, err := svc.SaveSource(context.Background(), "테스트 제목", "", "본문 내용")
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("파일 미생성: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(repo, "sources", "conversation") {
		t.Fatalf("저장 위치 = %q", filepath.Dir(path))
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -run TestService -v`
Expected: FAIL (`NewService` 미정의)

- [ ] **Step 3: 구현**

`internal/knowledge/service.go` 생성:

```go
package knowledge

import (
	"context"
	"strings"
	"time"
)

// summarizer 는 요약에 쓰는 텍스트 생성 능력이다(*gemini.Client 가 만족).
type summarizer interface {
	GenerateText(ctx context.Context, system, user string) (string, error)
}

// summarySystem 은 거칠게 추출된 대화를 지식 노트로 정리하는 지시문이다.
const summarySystem = `다음은 ChatGPT 대화를 거칠게 추출한 텍스트다(잡음·중복·UI 문구가 섞일 수 있다).
한 개발자의 개인 지식저장소에 넣을 간결한 한국어 요약 노트로 정리해라.
- 인사·잡담·중복·UI 문구는 버리고 핵심 개념과 결론만 남겨라.
- 마크다운으로 작성하고, ## 핵심 / ## 상세 같은 섹션을 자유롭게 구성해라.
- frontmatter(---)는 넣지 마라. 본문만 출력해라.`

// Service 는 공유 대화 추출→요약, 요약 저장을 묶는다.
type Service struct {
	sum      summarizer
	repoPath string
}

// NewService 는 Service 를 생성한다.
func NewService(sum summarizer, repoPath string) Service {
	return Service{sum: sum, repoPath: repoPath}
}

// Summarize 는 공유 링크 대화를 추출해 요약한다(제목 + 요약 본문). 저장하지 않는다.
func (s Service) Summarize(ctx context.Context, url string) (string, string, error) {
	conv, err := FetchConversation(ctx, url)
	if err != nil {
		return "", "", err
	}
	summary, err := s.sum.GenerateText(ctx, summarySystem, strings.Join(conv.Messages, "\n"))
	if err != nil {
		return "", "", err
	}
	return conv.Title, strings.TrimSpace(summary), nil
}

// SaveSource 는 (수정 반영된) 요약을 sources/ 에 저장한다.
func (s Service) SaveSource(_ context.Context, title, url, content string) (string, error) {
	today := time.Now().Format("2006-01-02")
	return WriteSource(s.repoPath, today, title, url, content)
}
```

- [ ] **Step 4: 테스트 통과 + 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/knowledge/ -v && go build ./...`
Expected: PASS, 빌드 성공

- [ ] **Step 5: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/knowledge/service.go internal/knowledge/service_test.go
git commit -m "feat(knowledge): 추출+요약+저장을 묶는 Service 추가"
```

---

### Task 6: agent — 지식 도구 2개 + 시스템 프롬프트

**Files:**
- Create: `internal/agent/knowledge_tools.go`
- Modify: `internal/agent/agent.go` (DefaultSystemPrompt)
- Test: `internal/agent/knowledge_tools_test.go`

**Interfaces:**
- Consumes: `Tool`, `objSchema`, `strSchema`, `strArg`(tools.go).
- Produces: `KnowledgePort` 인터페이스; `KnowledgeTools(port KnowledgePort) []Tool`(도구 `summarize_chatgpt_share`, `save_kb_source`). Task 7이 `KnowledgeTools` 사용. `knowledge.Service`가 `KnowledgePort` 를 구조적으로 만족.

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/knowledge_tools_test.go`:

```go
package agent

import (
	"context"
	"strings"
	"testing"
)

type fakeKnowledge struct {
	title, summary             string
	savedTitle, savedURL, savedContent string
	path                       string
}

func (f *fakeKnowledge) Summarize(_ context.Context, _ string) (string, string, error) {
	return f.title, f.summary, nil
}
func (f *fakeKnowledge) SaveSource(_ context.Context, title, url, content string) (string, error) {
	f.savedTitle, f.savedURL, f.savedContent = title, url, content
	return f.path, nil
}

func toolByName(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Decl.Name == name {
			return t
		}
	}
	return Tool{}
}

func TestKnowledge_summarizeTool_returnsSummaryNoSave(t *testing.T) {
	t.Parallel()
	fk := &fakeKnowledge{title: "고랭 장점 설명", summary: "## 핵심\n- 고루틴"}
	tool := toolByName(KnowledgeTools(fk), "summarize_chatgpt_share")
	out, err := tool.Run(context.Background(), map[string]any{"url": "https://chatgpt.com/share/x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "고랭 장점 설명") || !strings.Contains(out, "고루틴") {
		t.Fatalf("요약 응답 = %q", out)
	}
	if fk.savedContent != "" {
		t.Fatal("summarize 는 저장하면 안 됨")
	}
}

func TestKnowledge_saveTool_passesContent(t *testing.T) {
	t.Parallel()
	fk := &fakeKnowledge{path: "/kb/sources/conversation/2026-06-18-고랭-장점-설명.md"}
	tool := toolByName(KnowledgeTools(fk), "save_kb_source")
	out, err := tool.Run(context.Background(), map[string]any{
		"title": "고랭 장점 설명", "url": "https://chatgpt.com/share/x", "content": "## 핵심(수정됨)\n- 채널",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fk.savedContent != "## 핵심(수정됨)\n- 채널" || fk.savedTitle != "고랭 장점 설명" {
		t.Fatalf("저장 인자 = %q / %q", fk.savedTitle, fk.savedContent)
	}
	if !strings.Contains(out, fk.path) {
		t.Fatalf("저장 응답에 경로 없음: %q", out)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/agent/ -run TestKnowledge -v`
Expected: FAIL (`KnowledgeTools`/`KnowledgePort` 미정의)

- [ ] **Step 3: 구현 — knowledge_tools.go**

`internal/agent/knowledge_tools.go` 생성:

```go
package agent

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// KnowledgePort 는 지식 도구가 필요로 하는 작업이다(테스트에서 fake 주입).
type KnowledgePort interface {
	Summarize(ctx context.Context, url string) (title string, summary string, err error)
	SaveSource(ctx context.Context, title, url, content string) (path string, err error)
}

type knowledgeTools struct {
	port KnowledgePort
}

// KnowledgeTools 는 ChatGPT 공유링크 요약/저장 도구 목록을 만든다(둘 다 읽기형).
func KnowledgeTools(port KnowledgePort) []Tool {
	k := knowledgeTools{port: port}
	return []Tool{k.summarizeShare(), k.saveSource()}
}

func (k knowledgeTools) summarizeShare() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "summarize_chatgpt_share",
			Description: "ChatGPT 공유 링크(chatgpt.com/share/...)의 대화를 추출해 요약한다. 저장은 하지 않는다(보여주기만).",
			Parameters: objSchema(map[string]*genai.Schema{
				"url": strSchema("ChatGPT 공유 링크 URL"),
			}, "url"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			title, summary, err := k.port.Summarize(ctx, strArg(args, "url"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("제목: %s\n\n%s", title, summary), nil
		},
	}
}

func (k knowledgeTools) saveSource() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "save_kb_source",
			Description: "사용자가 요약을 확정(예: '저장해')하면 지식저장소 sources/ 에 저장한다. content 에는 현재 대화에서 보여준 (수정 반영된) 요약 본문 전체를 넣어라.",
			Parameters: objSchema(map[string]*genai.Schema{
				"title":   strSchema("문서 제목"),
				"content": strSchema("저장할 요약 본문 전체(마크다운, 수정 반영된 최종본)"),
				"url":     strSchema("출처 ChatGPT 공유 링크(있으면)"),
			}, "title", "content"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := k.port.SaveSource(ctx, strArg(args, "title"), strArg(args, "url"), strArg(args, "content"))
			if err != nil {
				return "", err
			}
			return "저장했어: " + path, nil
		},
	}
}
```

- [ ] **Step 4: 구현 — 시스템 프롬프트 추가**

`internal/agent/agent.go` 의 `DefaultSystemPrompt` 상수 맨 끝(백틱 닫기 ` 직전)에 아래 줄을 추가:

```
- ChatGPT 공유 링크(chatgpt.com/share/... 또는 chat.openai.com/share/...)를 정리/요약 요청과 함께 받으면 summarize_chatgpt_share 도구를 그 URL 로 호출한다. 요약을 보여준 뒤 **바로 저장하지 마라.**
- 사용자가 요약 수정을 요청하면(예: "더 짧게", "이 부분 빼") 도구 없이 대화로 직접 고쳐 다시 보여줘라.
- 사용자가 "저장/저장해/이대로 저장" 등으로 확정하면 그때 save_kb_source 를 호출하되, content 에는 현재 보여준 (수정 반영된) 요약 본문 전체를 넣어라.
```

- [ ] **Step 5: 테스트 통과 + 빌드 확인**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go test ./internal/agent/ -v && go build ./...`
Expected: PASS(기존 + 신규 지식 테스트), 빌드 성공

- [ ] **Step 6: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add internal/agent/knowledge_tools.go internal/agent/knowledge_tools_test.go internal/agent/agent.go
git commit -m "feat(agent): ChatGPT 공유링크 요약/저장 도구 + 프롬프트"
```

---

### Task 7: 조립 + 전체 검증 + 라이브 체크리스트

**Files:**
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `cfg.KnowledgeRepoPath`(Task 1), `knowledge.NewService`(Task 5), `agent.KnowledgeTools`(Task 6), 기존 `gemini.New`·`agent.HomeTools`·`agent.New`.

- [ ] **Step 1: main.go 조립**

`cmd/server/main.go` import 에 추가:

```go
	"github.com/Jongseong0111/jarvis/internal/knowledge"
```

`ag := agent.New(...)` 줄을 아래로 교체(지식 서비스 생성 + 도구 합치기):

```go
	knowledgeSvc := knowledge.NewService(geminiClient, cfg.KnowledgeRepoPath)
	tools := append(agent.HomeTools(home, cfg.NotionHomeURL, mapURL), agent.KnowledgeTools(knowledgeSvc)...)
	ag := agent.New(geminiClient, tools, "")
```

> `geminiClient`(*gemini.Client)는 `GenerateText` 를 가지므로 `knowledge` 의 `summarizer` 를, `knowledge.Service` 는 `agent.KnowledgePort` 를 구조적으로 만족한다(컴파일 시 검증됨).

- [ ] **Step 2: 전체 빌드/vet/테스트**

Run: `cd /Users/seonghyun/personal-agent/jarvis && go build ./... && go vet ./... && go test ./...`
Expected: 모두 성공, 전 패키지 ok

- [ ] **Step 3: 커밋**

```bash
cd /Users/seonghyun/personal-agent/jarvis
git add cmd/server/main.go
git commit -m "feat: 지식저장소 ChatGPT 공유링크 요약 조립(knowledge 도구 주입)"
```

- [ ] **Step 4: 서버 재시작 (사람이 수행)**

```bash
cd /Users/seonghyun/personal-agent/jarvis
pkill -f bin/jarvis 2>/dev/null
go build -o bin/jarvis ./cmd/server
nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
sleep 2 && tail -5 /tmp/jarvis.log
```

Expected: 로그에 "jarvis 시작"

- [ ] **Step 5: 라이브 검증 (Slack, 사람이 수행)**

1. `@jarvis 이 대화 정리해줘 <ChatGPT 공유링크>` → 요약이 슬랙에 옴(저장 아직 안 됨).
2. 이어서 "더 짧게" 또는 "고루틴 부분만 남겨" → 수정된 요약이 다시 옴.
3. "저장해" → `~/personal-agent/knowledge-base/sources/conversation/<날짜>-<slug>.md` 생성 확인(`git -C ~/personal-agent/knowledge-base status`).
4. 비공유/깨진 링크 → "대화를 못 가져왔어" 안내.
5. 사진/집정리 등 기존 기능 회귀 없음.

---

## Phase B 예고 (이번 범위 아님)

- Claude Code headless(`claude -p`)로 `kb-ingest` 실행 → 소스에서 개념별 concept 문서 분리.
- 슬랙 승인 버튼 → git diff 리뷰 → 커밋(+선택 push). 승인 전 커밋 금지.
- 느린 작업이라 비동기(고루틴 + 완료 시 슬랙 알림) 구조 도입.
- kb-ingest 스킬의 옛 경로(`acloset-agent/base/...`) → `~/personal-agent/knowledge-base` 수정.
