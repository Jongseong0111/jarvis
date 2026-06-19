# Dev Digest 아침 브리핑 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 매일 09:00에 GeekNews+HN 뉴스 3-5개(링크+한줄요약)와 계층형 개발 공부 주제 3-5개를 Slack으로 전송한다.

**Architecture:** `internal/devdigest` 패키지가 RSS/HN fetch + Gemini 생성을 담당하고, `agent.NewDevDigestBriefing`이 기존 스케줄러에 등록한다. 기존 Todoist 브리핑과 동일한 `scheduler.Job` + `domain.MessageSender` 패턴을 사용한다.

**Tech Stack:** Go stdlib (`net/http`, `encoding/xml`, `encoding/json`, `sync`), `google.golang.org/genai`, `github.com/slack-go/slack`

## Global Constraints

- 모듈: `github.com/Jongseong0111/jarvis`, Go 1.25
- 로깅: `pkg/log` slog 래퍼 (`log.FromContext`)
- 에러: `fmt.Errorf("...: %w", err)` — 커스텀 에러 타입 없음
- 테스트: `t.Parallel()`, httptest, fake 인터페이스, 정적 데이터
- 커밋: 한국어 메시지, `feat/fix/refactor/test/docs:` 접두

---

## 파일 구조

| 파일 | 역할 |
|---|---|
| `internal/devdigest/fetcher.go` | GeekNews RSS + HN API 병렬 fetch |
| `internal/devdigest/fetcher_test.go` | httptest 서버로 RSS/HN 모킹 |
| `internal/devdigest/digest.go` | Gemini 프롬프트 빌드 + JSON 파싱, `Generator` 인터페이스 |
| `internal/devdigest/digest_test.go` | fake generator로 포맷 검증 |
| `internal/agent/devdigest_briefing.go` | `NewDevDigestBriefing` 스케줄러 잡 팩토리 + Slack 포맷 |
| `internal/agent/devdigest_briefing_test.go` | fake fetcher/generator + capSender 재사용 |
| `pkg/config/config.go` | `DigestTime`, `DigestRSSURLs` 필드 추가, `parseCommaList` |
| `internal/scheduler/scheduler.go` | `Job.Timeout` 필드 추가 (0=30s 기본) |
| `cmd/server/main.go` | `startBriefings` 에 digest 잡 등록 |
| `cmd/sendbrief/main.go` | `-kind=digest` 케이스 추가 |

---

### Task 1: Config 확장 + Scheduler Timeout 필드

**Files:**
- Modify: `pkg/config/config.go`
- Modify: `internal/scheduler/scheduler.go`

**Interfaces:**
- Produces:
  - `config.Config.DigestTime string` — `"HH:MM"`, 기본 `"09:00"`
  - `config.Config.DigestRSSURLs []string` — 추가 RSS URL 목록
  - `scheduler.Job.Timeout time.Duration` — 0이면 30s 기본

- [ ] **Step 1: scheduler.Job 에 Timeout 필드 추가**

`internal/scheduler/scheduler.go` 의 `Job` 구조체와 `fire` 메서드를 수정한다:

```go
// Job 은 매일 지정 시각에 실행할 작업이다.
type Job struct {
	Name    string
	Hour    int
	Min     int
	TZ      *time.Location
	Timeout time.Duration // 0 이면 30s 기본
	Fn      func(ctx context.Context)
}
```

`fire` 메서드 안의 하드코딩된 `30*time.Second` 를:

```go
func (s *Scheduler) fire(ctx context.Context, j Job, logger *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("스케줄 작업 패닉", "job", j.Name, "recover", r)
		}
	}()
	timeout := j.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	j.Fn(runCtx)
}
```

- [ ] **Step 2: config.go 에 DigestTime, DigestRSSURLs 추가**

`pkg/config/config.go` 의 `Config` 구조체에 추가:

```go
DigestTime    string   // "HH:MM", 기본 "09:00"
DigestRSSURLs []string // 추가 RSS 피드 URL 목록
```

`New()` 함수의 `Config` 초기화에 추가:

```go
DigestTime:    getenv("DIGEST_TIME", "09:00"),
DigestRSSURLs: parseCommaList(os.Getenv("DIGEST_RSS_URLS")),
```

파일 하단에 헬퍼 함수 추가 (`expandHome` 다음):

```go
// parseCommaList 는 쉼표로 구분된 문자열을 슬라이스로 파싱한다(빈 값 제거).
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
```

- [ ] **Step 3: 빌드 확인**

```bash
go build ./...
```

Expected: 에러 없음

- [ ] **Step 4: 기존 테스트 확인**

```bash
go test ./internal/scheduler/... ./pkg/config/...
```

Expected: PASS (기존 테스트는 `Timeout` 필드를 0으로 두므로 30s 기본값 적용)

- [ ] **Step 5: 커밋**

```bash
git add pkg/config/config.go internal/scheduler/scheduler.go
git commit -m "feat(config,scheduler): DigestTime·DigestRSSURLs 설정 + Job.Timeout 필드"
```

---

### Task 2: Fetcher — GeekNews RSS + HN API

**Files:**
- Create: `internal/devdigest/fetcher.go`
- Create: `internal/devdigest/fetcher_test.go`

**Interfaces:**
- Produces:
  - `devdigest.NewsItem{Title, URL, Desc string}`
  - `devdigest.Fetcher` 인터페이스: `Fetch(ctx context.Context) ([]NewsItem, error)`
  - `devdigest.NewFetcher(extraURLs []string) *MultiFetcher`

- [ ] **Step 1: fetcher_test.go 작성**

```go
package devdigest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item><title>Go 1.25 출시</title><link>https://go.dev/blog/go1.25</link><description>새 기능 요약</description></item>
  <item><title>Rust 2025 에디션</title><link>https://blog.rust-lang.org/2025</link><description>Rust 업데이트</description></item>
</channel></rss>`

func newTestFetcher(rssURLs []string, hnTopURL, hnItemBase string) *MultiFetcher {
	return &MultiFetcher{
		httpClient:    &http.Client{},
		rssURLs:       rssURLs,
		hnTopURL:      hnTopURL,
		hnItemBaseURL: hnItemBase,
	}
}

func TestFetcher_RSS(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, sampleRSS)
	}))
	defer srv.Close()

	f := newTestFetcher([]string{srv.URL}, "invalid://hn-top", "invalid://hn-item/%d")
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("기대 2건, got %d: %+v", len(items), items)
	}
	if items[0].Title != "Go 1.25 출시" || items[0].URL != "https://go.dev/blog/go1.25" {
		t.Fatalf("첫 아이템 불일치: %+v", items[0])
	}
}

func TestFetcher_HN(t *testing.T) {
	t.Parallel()

	itemSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1,"title":"HN 기사","url":"https://example.com","type":"story","score":100}`)
	}))
	defer itemSrv.Close()

	topSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[1]`)
	}))
	defer topSrv.Close()

	f := newTestFetcher(nil, topSrv.URL, itemSrv.URL+"/%d")
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Title == "HN 기사" {
			found = true
		}
	}
	if !found {
		t.Fatalf("HN 아이템 없음: %+v", items)
	}
}

func TestFetcher_AllSourcesFail(t *testing.T) {
	t.Parallel()
	f := newTestFetcher([]string{"http://127.0.0.1:1"}, "http://127.0.0.1:1", "http://127.0.0.1:1/%d")
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Fatal("모든 소스 실패 시 error 기대")
	}
}
```

- [ ] **Step 2: 테스트 실행해서 실패 확인**

```bash
go test ./internal/devdigest/ -run TestFetcher -v 2>&1 | head -20
```

Expected: 컴파일 에러 또는 FAIL (`MultiFetcher` 미정의)

- [ ] **Step 3: fetcher.go 구현**

```go
// Package devdigest 는 개발자 아침 다이제스트(뉴스+공부주제)를 생성한다.
package devdigest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	defaultGeekNewsRSS  = "https://news.hada.io/rss"
	defaultHNTopURL     = "https://hacker-news.firebaseio.com/v0/topstories.json"
	defaultHNItemBase   = "https://hacker-news.firebaseio.com/v0/item/%d.json"
	maxHNFetch          = 30
	maxRSSItemsPerFeed  = 30
)

// NewsItem 은 뉴스 피드에서 가져온 기사 하나다.
type NewsItem struct {
	Title string
	URL   string
	Desc  string
}

// Fetcher 는 뉴스 아이템을 가져오는 인터페이스다.
type Fetcher interface {
	Fetch(ctx context.Context) ([]NewsItem, error)
}

// MultiFetcher 는 RSS 피드들 + HN API 에서 뉴스를 병렬로 가져온다.
type MultiFetcher struct {
	httpClient    *http.Client
	rssURLs       []string
	hnTopURL      string
	hnItemBaseURL string
}

// NewFetcher 는 GeekNews RSS + HN + 추가 RSS URL 로 MultiFetcher 를 생성한다.
func NewFetcher(extraURLs []string) *MultiFetcher {
	rssURLs := append([]string{defaultGeekNewsRSS}, extraURLs...)
	return &MultiFetcher{
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		rssURLs:       rssURLs,
		hnTopURL:      defaultHNTopURL,
		hnItemBaseURL: defaultHNItemBase,
	}
}

// Fetch 는 등록된 모든 소스에서 뉴스를 병렬로 가져온다.
// 개별 소스 실패는 건너뛴다. 모든 소스가 실패하면 error 를 반환한다.
func (f *MultiFetcher) Fetch(ctx context.Context) ([]NewsItem, error) {
	var (
		mu    sync.Mutex
		items []NewsItem
		errs  []error
		wg    sync.WaitGroup
	)

	collect := func(got []NewsItem, err error, label string) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
			return
		}
		items = append(items, got...)
	}

	for _, u := range f.rssURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			got, err := f.fetchRSS(ctx, url)
			collect(got, err, url)
		}(u)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err := f.fetchHN(ctx)
		collect(got, err, "hn")
	}()

	wg.Wait()

	if len(items) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("모든 뉴스 소스 fetch 실패: %v", errs)
	}
	return items, nil
}

type rssXML struct {
	Items []struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
}

func (f *MultiFetcher) fetchRSS(ctx context.Context, url string) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed rssXML
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("RSS 파싱 실패: %w", err)
	}

	var out []NewsItem
	for i, it := range feed.Items {
		if i >= maxRSSItemsPerFeed {
			break
		}
		out = append(out, NewsItem{Title: it.Title, URL: it.Link, Desc: it.Description})
	}
	return out, nil
}

func (f *MultiFetcher) fetchHN(ctx context.Context) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.hnTopURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("HN top stories 파싱 실패: %w", err)
	}
	if len(ids) > maxHNFetch {
		ids = ids[:maxHNFetch]
	}

	var (
		mu    sync.Mutex
		items []NewsItem
		wg    sync.WaitGroup
	)
	for _, id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			it, err := f.fetchHNItem(ctx, id)
			if err != nil || it == nil {
				return
			}
			mu.Lock()
			items = append(items, *it)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return items, nil
}

type hnItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

func (f *MultiFetcher) fetchHNItem(ctx context.Context, id int) (*NewsItem, error) {
	url := fmt.Sprintf(f.hnItemBaseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var it hnItem
	if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
		return nil, err
	}
	if it.Type != "story" || it.URL == "" {
		return nil, nil // 링크 없는 Ask HN 등 스킵
	}
	return &NewsItem{Title: it.Title, URL: it.URL}, nil
}
```

- [ ] **Step 4: 테스트 실행**

```bash
go test ./internal/devdigest/ -run TestFetcher -v -race
```

Expected: 3개 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/devdigest/
git commit -m "feat(devdigest): MultiFetcher — GeekNews RSS + HN API 병렬 fetch"
```

---

### Task 3: Digest Generator — Gemini 뉴스 선별 + 공부주제 생성

**Files:**
- Create: `internal/devdigest/digest.go`
- Create: `internal/devdigest/digest_test.go`

**Interfaces:**
- Consumes: `devdigest.NewsItem` (Task 2)
- Produces:
  - `devdigest.NewsResult{Title, URL, Summary string}`
  - `devdigest.DigestResult{News []NewsResult, Domain string, Topics []string}`
  - `devdigest.Generator` 인터페이스: `Generate(ctx, []NewsItem) (DigestResult, error)`
  - `devdigest.NewGenerator(client *gemini.Client) *GeminiGenerator`

- [ ] **Step 1: digest_test.go 작성**

```go
package devdigest

import (
	"context"
	"strings"
	"testing"
)

func TestParseResponse_valid(t *testing.T) {
	t.Parallel()
	raw := `{"news":[{"title":"Go 1.25","url":"https://go.dev","summary":"새 기능 출시"}],"domain":"언어","topics":["언어 → Go → goroutine 스케줄러"]}`
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "언어" {
		t.Fatalf("domain=%q", result.Domain)
	}
	if len(result.News) != 1 || result.News[0].URL != "https://go.dev" {
		t.Fatalf("news=%+v", result.News)
	}
	if len(result.Topics) != 1 || !strings.Contains(result.Topics[0], "goroutine") {
		t.Fatalf("topics=%+v", result.Topics)
	}
}

func TestParseResponse_markdownWrapped(t *testing.T) {
	t.Parallel()
	raw := "```json\n{\"news\":[],\"domain\":\"AI\",\"topics\":[\"AI → LLM → Transformer 구조\"]}\n```"
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "AI" {
		t.Fatalf("domain=%q", result.Domain)
	}
}

func TestParseResponse_invalid(t *testing.T) {
	t.Parallel()
	_, err := parseResponse("not json")
	if err == nil {
		t.Fatal("잘못된 JSON 에 error 기대")
	}
}

func TestBuildPrompt_containsItems(t *testing.T) {
	t.Parallel()
	items := []NewsItem{
		{Title: "기사A", URL: "https://a.com", Desc: "설명A"},
	}
	p := buildPrompt(items)
	if !strings.Contains(p, "기사A") || !strings.Contains(p, "https://a.com") {
		t.Fatalf("prompt=%q", p)
	}
}
```

- [ ] **Step 2: 테스트 실행해서 실패 확인**

```bash
go test ./internal/devdigest/ -run TestParse -v 2>&1 | head -10
```

Expected: FAIL (`parseResponse` 미정의)

- [ ] **Step 3: digest.go 구현**

```go
package devdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/internal/gemini"
)

// domains 는 공부 주제 도메인 목록이다. Gemini 가 매일 하나를 선택한다.
var domains = []string{
	"언어", "웹·백엔드", "데이터베이스", "인프라",
	"데이터", "운영체제", "네트워크",
	"자료구조·알고리즘", "개발도구", "AI", "기타",
}

// NewsResult 는 Gemini 가 선별한 뉴스 기사 하나다.
type NewsResult struct {
	Title   string
	URL     string
	Summary string
}

// DigestResult 는 Gemini 가 생성한 뉴스+공부주제 다이제스트다.
type DigestResult struct {
	News   []NewsResult
	Domain string
	Topics []string
}

// Generator 는 뉴스 아이템에서 다이제스트를 생성하는 인터페이스다.
type Generator interface {
	Generate(ctx context.Context, items []NewsItem) (DigestResult, error)
}

// GeminiGenerator 는 Gemini 를 사용해 다이제스트를 생성한다.
type GeminiGenerator struct {
	client *gemini.Client
}

// NewGenerator 는 GeminiGenerator 를 생성한다.
func NewGenerator(client *gemini.Client) *GeminiGenerator {
	return &GeminiGenerator{client: client}
}

// Generate 는 뉴스 후보 목록을 Gemini 에 보내 선별·요약 + 공부주제를 생성한다.
func (g *GeminiGenerator) Generate(ctx context.Context, items []NewsItem) (DigestResult, error) {
	raw, err := g.client.GenerateText(ctx, systemPrompt, buildPrompt(items))
	if err != nil {
		return DigestResult{}, fmt.Errorf("gemini 다이제스트 생성 실패: %w", err)
	}
	return parseResponse(raw)
}

const systemPrompt = `너는 개발자를 위한 아침 다이제스트를 만드는 어시스턴트다. 반드시 JSON 으로만 응답하라. 마크다운 코드블록 없이 순수 JSON 만 출력하라.`

func buildPrompt(items []NewsItem) string {
	var sb strings.Builder
	sb.WriteString("[뉴스 후보 목록]\n")
	for i, it := range items {
		sb.WriteString(fmt.Sprintf("%d. %s | %s | %s\n", i+1, it.Title, it.URL, it.Desc))
	}
	sb.WriteString("\n[작업]\n")
	sb.WriteString("1. 위 목록에서 개발자에게 가장 흥미로운 항목 3-5개를 골라라.\n")
	sb.WriteString("   - 실제 기술 내용 우선 (채용/마케팅/이벤트 제외)\n")
	sb.WriteString("   - 각 항목: title(원문 유지), url(원문), summary(한국어 한줄 요약)\n\n")
	sb.WriteString("2. 오늘의 개발 공부 주제를 생성하라.\n")
	sb.WriteString("   - 아래 도메인 중 하나 선택: " + strings.Join(domains, " / ") + "\n")
	sb.WriteString("   - 인프라 선택 시 Kafka·RabbitMQ 같은 메시징 시스템도 포함 가능\n")
	sb.WriteString("   - 계층형 주제 3-5개: \"도메인 → 중분류 → 구체 개념\" 형식\n")
	sb.WriteString("   - 예: \"데이터베이스 → Vector DB → HNSW 인덱스 구조\"\n\n")
	sb.WriteString("JSON: {\"news\":[{\"title\":\"...\",\"url\":\"...\",\"summary\":\"...\"}],\"domain\":\"...\",\"topics\":[\"...\"]}")
	return sb.String()
}

type digestJSON struct {
	News []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Summary string `json:"summary"`
	} `json:"news"`
	Domain string   `json:"domain"`
	Topics []string `json:"topics"`
}

func parseResponse(raw string) (DigestResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var dj digestJSON
	if err := json.Unmarshal([]byte(raw), &dj); err != nil {
		return DigestResult{}, fmt.Errorf("응답 JSON 파싱 실패: %w", err)
	}

	result := DigestResult{Domain: dj.Domain, Topics: dj.Topics}
	for _, n := range dj.News {
		result.News = append(result.News, NewsResult{Title: n.Title, URL: n.URL, Summary: n.Summary})
	}
	return result, nil
}
```

- [ ] **Step 4: 테스트 실행**

```bash
go test ./internal/devdigest/ -v -race
```

Expected: 6개 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/devdigest/digest.go internal/devdigest/digest_test.go
git commit -m "feat(devdigest): GeminiGenerator — 뉴스 선별·요약 + 계층형 공부주제 생성"
```

---

### Task 4: Briefing Job — Slack 포맷 + 스케줄러 잡 팩토리

**Files:**
- Create: `internal/agent/devdigest_briefing.go`
- Create: `internal/agent/devdigest_briefing_test.go`

**Interfaces:**
- Consumes:
  - `devdigest.Fetcher` (Task 2)
  - `devdigest.Generator` (Task 3)
  - `domain.MessageSender`
- Produces:
  - `agent.NewDevDigestBriefing(fetcher, generator, sender, channel) func(ctx context.Context)`

- [ ] **Step 1: devdigest_briefing_test.go 작성**

`capSender` 는 `todoist_briefing_test.go` 에 이미 정의됨(같은 `package agent`). 재사용한다.

```go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

type fakeFetcher struct {
	items []devdigest.NewsItem
	err   error
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]devdigest.NewsItem, error) {
	return f.items, f.err
}

type fakeGenerator struct {
	result devdigest.DigestResult
	err    error
}

func (g *fakeGenerator) Generate(_ context.Context, _ []devdigest.NewsItem) (devdigest.DigestResult, error) {
	return g.result, g.err
}

func TestNewDevDigestBriefing_sendsFormattedMessage(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{items: []devdigest.NewsItem{{Title: "기사", URL: "https://ex.com", Desc: "설명"}}}
	generator := &fakeGenerator{result: devdigest.DigestResult{
		News:   []devdigest.NewsResult{{Title: "Go 1.25", URL: "https://go.dev", Summary: "새 기능"}},
		Domain: "언어",
		Topics: []string{"언어 → Go → goroutine 스케줄러"},
	}}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C1")(context.Background())

	if len(sender.sent) != 1 {
		t.Fatalf("메시지 1건 기대: %+v", sender.sent)
	}
	text := sender.sent[0].Text
	if !strings.Contains(text, "개발 소식") {
		t.Fatalf("뉴스 헤더 없음: %q", text)
	}
	if !strings.Contains(text, "Go 1.25") || !strings.Contains(text, "https://go.dev") {
		t.Fatalf("뉴스 내용 없음: %q", text)
	}
	if !strings.Contains(text, "공부 주제") {
		t.Fatalf("공부 주제 헤더 없음: %q", text)
	}
	if !strings.Contains(text, "goroutine") {
		t.Fatalf("주제 내용 없음: %q", text)
	}
	if sender.sent[0].ChannelID != "C1" {
		t.Fatalf("channel=%q", sender.sent[0].ChannelID)
	}
}

func TestNewDevDigestBriefing_fetchFailStillGenerates(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{err: fmt.Errorf("네트워크 오류")}
	generator := &fakeGenerator{result: devdigest.DigestResult{
		Domain: "AI",
		Topics: []string{"AI → LLM → Transformer"},
	}}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C2")(context.Background())
	// fetch 실패해도 generate 는 시도(빈 items 로)하고 전송
	if len(sender.sent) != 1 {
		t.Fatalf("fetch 실패해도 메시지 기대: %+v", sender.sent)
	}
}

func TestNewDevDigestBriefing_generateFailSilent(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{}
	generator := &fakeGenerator{err: fmt.Errorf("gemini 오류")}
	sender := &capSender{}
	NewDevDigestBriefing(fetcher, generator, sender, "C3")(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("generate 실패 시 무음 기대: %+v", sender.sent)
	}
}
```

- [ ] **Step 2: 테스트 실행해서 실패 확인**

```bash
go test ./internal/agent/ -run TestNewDevDigest -v 2>&1 | head -20
```

Expected: FAIL (컴파일 에러 또는 `NewDevDigestBriefing` 미정의)

- [ ] **Step 3: devdigest_briefing.go 구현**

`devdigest_briefing_test.go` 에서 `fmt` 패키지를 import 해야 하므로 test 파일 상단에 `"fmt"` 를 추가한다.

```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/devdigest"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// NewDevDigestBriefing 은 개발 다이제스트 브리핑 스케줄러 잡을 만든다.
// fetch 실패 시 빈 목록으로 generate 를 시도한다. generate 실패 시 무음.
func NewDevDigestBriefing(fetcher devdigest.Fetcher, generator devdigest.Generator, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		items, err := fetcher.Fetch(ctx)
		if err != nil {
			log.FromContext(ctx).Error("뉴스 fetch 실패", "error", err)
			// 빈 items 로 공부주제만이라도 생성 시도
		}

		result, err := generator.Generate(ctx, items)
		if err != nil {
			log.FromContext(ctx).Error("다이제스트 생성 실패", "error", err)
			return
		}

		sendText(ctx, sender, channel, formatDigest(result))
	}
}

func formatDigest(r devdigest.DigestResult) string {
	var sb strings.Builder

	sb.WriteString("📰 *오늘의 개발 소식*\n")
	for _, n := range r.News {
		sb.WriteString(fmt.Sprintf("• <%s|%s> — %s\n", n.URL, n.Title, n.Summary))
	}

	sb.WriteString(fmt.Sprintf("\n📚 *오늘의 공부 주제*  _(도메인: %s)_\n", r.Domain))
	for _, topic := range r.Topics {
		sb.WriteString("• " + topic + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
```

- [ ] **Step 4: 테스트 실행**

```bash
go test ./internal/agent/ -run TestNewDevDigest -v -race
```

Expected: 3개 PASS

- [ ] **Step 5: 전체 테스트 확인**

```bash
go test ./... -race
```

Expected: PASS (기존 포함)

- [ ] **Step 6: 커밋**

```bash
git add internal/agent/devdigest_briefing.go internal/agent/devdigest_briefing_test.go
git commit -m "feat(agent): NewDevDigestBriefing — Slack 포맷 + 스케줄러 잡 팩토리"
```

---

### Task 5: 조립 — server + sendbrief 등록

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `cmd/sendbrief/main.go`

**Interfaces:**
- Consumes: 모든 이전 Task 의 결과물

- [ ] **Step 1: cmd/server/main.go 의 startBriefings 수정**

`startBriefings` 함수 시그니처에 `geminiClient *gemini.Client` 를 추가하고, digest 잡을 등록한다.

현재 시그니처:
```go
func startBriefings(ctx context.Context, cfg config.Config, client agent.TodoistPort, sender domain.MessageSender, logger *slog.Logger) error {
```

변경 후:
```go
func startBriefings(ctx context.Context, cfg config.Config, todoistClient agent.TodoistPort, geminiClient *gemini.Client, sender domain.MessageSender, logger *slog.Logger) error {
```

함수 body 안의 `sched.Register(evening)` 다음 줄에 추가:

```go
	// digest 브리핑
	dh, dm, err := config.ParseHHMM(cfg.DigestTime)
	if err != nil {
		return fmt.Errorf("다이제스트 시각: %w", err)
	}
	fetcher := devdigest.NewFetcher(cfg.DigestRSSURLs)
	generator := devdigest.NewGenerator(geminiClient)
	sched.Register(scheduler.Job{
		Name:    "dev-digest",
		Hour:    dh,
		Min:     dm,
		TZ:      tz,
		Timeout: 60 * time.Second,
		Fn:      agent.NewDevDigestBriefing(fetcher, generator, sender, cfg.TodoistBriefingChannel),
	})
```

함수 끝의 로그를 갱신:
```go
	logger.Info("브리핑 스케줄러 기동",
		"morning", cfg.TodoistMorning,
		"evening", cfg.TodoistEvening,
		"digest", cfg.DigestTime,
		"tz", cfg.TodoistTZ,
	)
```

`main()` 안의 `startBriefings` 호출을 수정 (`geminiClient` 추가):
```go
if err := startBriefings(ctx, cfg, todoistClient, geminiClient, client, logger); err != nil {
```

파일 상단 import 에 추가:
```go
"github.com/Jongseong0111/jarvis/internal/devdigest"
```

- [ ] **Step 2: cmd/sendbrief/main.go 에 digest 케이스 추가**

현재 `main()` 상단의 import 목록에 추가:
```go
"github.com/Jongseong0111/jarvis/internal/devdigest"
"github.com/Jongseong0111/jarvis/internal/gemini"
```

`slackClient` 생성 다음에 Gemini 클라이언트 생성 추가:
```go
	geminiClient := gemini.New(cfg.GeminiAPIKey, cfg.GeminiModel)
```

`switch *kind` 블록에 케이스 추가:
```go
	case "digest":
		fetcher := devdigest.NewFetcher(cfg.DigestRSSURLs)
		generator := devdigest.NewGenerator(geminiClient)
		agent.NewDevDigestBriefing(fetcher, generator, slackClient, ch)(ctx)
```

- [ ] **Step 3: 빌드 확인**

```bash
go build ./...
```

Expected: 에러 없음

- [ ] **Step 4: 전체 테스트**

```bash
go test ./... -race
```

Expected: PASS

- [ ] **Step 5: 라이브 전송 테스트**

```bash
go run ./cmd/sendbrief -kind=digest
```

Expected: Slack 채널에 뉴스 + 공부주제 메시지 전송됨.  
로그: `브리핑 전송 중 kind=digest` → `전송 완료`

- [ ] **Step 6: 바이너리 재빌드 + 서버 재시작**

```bash
go build -o bin/jarvis ./cmd/server
pkill -f bin/jarvis
nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
tail -5 /tmp/jarvis.log
```

Expected 로그:
```
브리핑 스케줄러 기동 morning=08:00 evening=21:00 digest=09:00 tz=Asia/Seoul
jarvis 시작 env=local
```

- [ ] **Step 7: 커밋**

```bash
git add cmd/server/main.go cmd/sendbrief/main.go
git commit -m "feat(server): dev digest 잡 조립 — 09:00 스케줄러 등록 + sendbrief -kind=digest"
```
