# LLM 비용 추적 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** jarvis가 호출하는 모든 LLM(Gemini, Claude Code) 비용을 `~/.jarvis/usage.jsonl`에 영구 기록하고, `list_usage` 도구로 기간별(source/model/feature) 집계를 슬랙에서 조회한다.

**Architecture:** 새 `internal/usage` 패키지가 append-only JSONL 기록과 집계를 담당한다. `usage.Recorder`를 `gemini.Client`와 `claudecode.CLIRunner`에 sink로 주입해 호출 지점 수정 없이 모든 LLM 호출을 자동 기록한다. feature 라벨은 `context` 값으로 전달한다.

**Tech Stack:** Go 1.25, stdlib만(`os`/`encoding/json`/`sync`/`time`/`context`), 기존 `google.golang.org/genai`.

## Global Constraints

- Go module `github.com/Jongseong0111/jarvis`, go 1.25.5.
- Clean Architecture: 도메인/구현 분리, constructor 주입(`New...`), value receiver 선호.
- 한국어 주석/커밋. 로깅 = stdlib `slog`(`pkg/log`). 에러 = `fmt.Errorf("...: %w", err)`.
- 테스트 = table-driven + `t.Parallel()` + 정적 시간 주입(`now func() time.Time`).
- 비용 기록은 **best-effort**: 실패해도 패닉/에러 전파 없이 `slog` 경고만. 사용자 요청 흐름을 절대 막지 않는다.
- 의존성 0 추가(usage 패키지는 stdlib만).
- Gemini 가격(USD/1M tokens): `gemini-2.5-flash` = in **0.30** / out **2.50**, `gemini-2.5-flash-lite` = in **0.10** / out **0.40**. (출처: ai.google.dev/gemini-api/docs/pricing, 2026-06)
- genai 토큰 필드: `resp.UsageMetadata.PromptTokenCount`(int32) = input, `.CandidatesTokenCount`(int32) = output. `UsageMetadata`는 nil 가능.
- Claude CLI(`--output-format json`) 필드: `total_cost_usd`(float), `usage.input_tokens`(int), `usage.output_tokens`(int). top-level `model` 없음 → `"claude"` 라벨.

---

### Task 1: usage 가격표

**Files:**
- Create: `internal/usage/pricing.go`
- Test: `internal/usage/pricing_test.go`

**Interfaces:**
- Produces: `func geminiCost(model string, inTk, outTk int) float64` (패키지 내부, 소문자).

- [ ] **Step 1: 실패 테스트 작성**

`internal/usage/pricing_test.go`:
```go
package usage

import (
	"math"
	"testing"
)

func TestGeminiCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		model      string
		inTk, outTk int
		want       float64
	}{
		{"flash", "gemini-2.5-flash", 1_000_000, 1_000_000, 0.30 + 2.50},
		{"flash-lite", "gemini-2.5-flash-lite", 1_000_000, 1_000_000, 0.10 + 0.40},
		{"flash partial", "gemini-2.5-flash", 2000, 100, 0.30*2000/1e6 + 2.50*100/1e6},
		{"unknown model -> 0", "gemini-9.9-ultra", 5000, 5000, 0},
		{"zero tokens", "gemini-2.5-flash", 0, 0, 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := geminiCost(tt.model, tt.inTk, tt.outTk)
			if math.Abs(got-tt.want) > 1e-12 {
				t.Fatalf("geminiCost(%q,%d,%d) = %v, want %v", tt.model, tt.inTk, tt.outTk, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -run TestGeminiCost -v`
Expected: FAIL (`undefined: geminiCost`).

- [ ] **Step 3: 구현**

`internal/usage/pricing.go`:
```go
// Package usage 는 LLM 호출 비용을 JSONL 로 기록하고 집계한다.
package usage

// price 는 모델별 100만 토큰당 USD 단가다.
type price struct{ inPer1M, outPer1M float64 }

// geminiPrices 는 Gemini 모델별 가격표다(출처: ai.google.dev/gemini-api/docs/pricing, 2026-06).
var geminiPrices = map[string]price{
	"gemini-2.5-flash":      {inPer1M: 0.30, outPer1M: 2.50},
	"gemini-2.5-flash-lite": {inPer1M: 0.10, outPer1M: 0.40},
}

// geminiCost 는 모델·토큰 수로 USD 비용을 계산한다. 모르는 모델은 0(토큰은 별도 기록됨).
func geminiCost(model string, inTk, outTk int) float64 {
	p, ok := geminiPrices[model]
	if !ok {
		return 0
	}
	return p.inPer1M*float64(inTk)/1e6 + p.outPer1M*float64(outTk)/1e6
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -run TestGeminiCost -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/usage/pricing.go internal/usage/pricing_test.go
git commit -m "feat(usage): Gemini 가격표 + 비용 계산"
```

---

### Task 2: Record + Recorder 기록 + feature context

**Files:**
- Create: `internal/usage/recorder.go`
- Create: `internal/usage/context.go`
- Test: `internal/usage/recorder_test.go`

**Interfaces:**
- Consumes: `geminiCost` (Task 1).
- Produces:
  - `type Record struct { Ts, Source, Feature, Model string; InputTk, OutputTk int; CostUSD float64 }` (json 태그 아래 코드 참조).
  - `func NewRecorder(path string) *Recorder`
  - `func (r *Recorder) LogGemini(feature, model string, inputTk, outputTk int)`
  - `func (r *Recorder) LogClaude(feature, model string, inputTk, outputTk int, costUSD float64)`
  - `func WithFeature(ctx context.Context, feature string) context.Context`
  - `func FeatureFromContext(ctx context.Context) string` (없으면 `""`)
  - `Recorder.now func() time.Time` 필드(테스트 주입용, 기본 `time.Now`).

- [ ] **Step 1: 실패 테스트 작성**

`internal/usage/recorder_test.go`:
```go
package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixedRecorder(t *testing.T) (*Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "usage.jsonl") // 중간 디렉터리 자동 생성 확인
	r := NewRecorder(path)
	r.now = func() time.Time { return time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC) }
	return r, path
}

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	var recs []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal %q: %v", sc.Text(), err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestRecorder_LogGemini(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	r.LogGemini("agent", "gemini-2.5-flash", 2000, 100)
	recs := readRecords(t, path)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	got := recs[0]
	want := Record{
		Ts: "2026-06-22T14:00:00Z", Source: "gemini", Feature: "agent",
		Model: "gemini-2.5-flash", InputTk: 2000, OutputTk: 100,
		CostUSD: 0.30*2000/1e6 + 2.50*100/1e6,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRecorder_LogClaude(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	r.LogClaude("kb", "claude", 5880, 5, 0.116938)
	recs := readRecords(t, path)
	if len(recs) != 1 || recs[0].Source != "claude" || recs[0].CostUSD != 0.116938 {
		t.Fatalf("unexpected: %+v", recs)
	}
}

func TestRecorder_ConcurrentAppend(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.LogGemini("agent", "gemini-2.5-flash", 10, 1) }()
	}
	wg.Wait()
	if recs := readRecords(t, path); len(recs) != 50 {
		t.Fatalf("want 50 records, got %d", len(recs))
	}
}

func TestRecorder_BestEffortNoPanic(t *testing.T) {
	t.Parallel()
	// 디렉터리로 만들 수 없는 경로(부모가 파일) → 기록 실패해도 패닉/중단 없어야 함
	bad := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(filepath.Join(bad, "usage.jsonl"))
	r.LogGemini("agent", "gemini-2.5-flash", 10, 1) // 패닉 없이 반환되면 통과
}

func TestFeatureContext(t *testing.T) {
	t.Parallel()
	if got := FeatureFromContext(context.Background()); got != "" {
		t.Fatalf("empty ctx want \"\", got %q", got)
	}
	ctx := WithFeature(context.Background(), "knowledge")
	if got := FeatureFromContext(ctx); got != "knowledge" {
		t.Fatalf("want knowledge, got %q", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -run 'TestRecorder|TestFeatureContext' -v`
Expected: FAIL (`undefined: NewRecorder` 등).

- [ ] **Step 3: 구현 (recorder.go)**

`internal/usage/recorder.go`:
```go
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record 는 usage.jsonl 한 줄(= LLM API 호출 1건)이다.
type Record struct {
	Ts       string  `json:"ts"`
	Source   string  `json:"source"`  // "gemini" | "claude"
	Feature  string  `json:"feature"` // agent|vision|knowledge|digest|kb
	Model    string  `json:"model"`
	InputTk  int     `json:"input_tk"`
	OutputTk int     `json:"output_tk"`
	CostUSD  float64 `json:"cost_usd"`
}

// Recorder 는 LLM 호출 비용을 append-only JSONL 로 기록한다(동시 쓰기 안전).
type Recorder struct {
	path string
	mu   sync.Mutex
	now  func() time.Time
}

// NewRecorder 는 path(JSONL 파일)에 기록하는 Recorder 를 만든다.
func NewRecorder(path string) *Recorder {
	return &Recorder{path: path, now: time.Now}
}

// LogGemini 는 Gemini 호출 1건을 기록한다(가격표로 cost 계산).
func (r *Recorder) LogGemini(feature, model string, inputTk, outputTk int) {
	r.append(Record{
		Source: "gemini", Feature: feature, Model: model,
		InputTk: inputTk, OutputTk: outputTk, CostUSD: geminiCost(model, inputTk, outputTk),
	})
}

// LogClaude 는 Claude 호출 1건을 기록한다(cost 는 CLI 가 직접 제공).
func (r *Recorder) LogClaude(feature, model string, inputTk, outputTk int, costUSD float64) {
	r.append(Record{
		Source: "claude", Feature: feature, Model: model,
		InputTk: inputTk, OutputTk: outputTk, CostUSD: costUSD,
	})
}

// append 는 Record 를 JSONL 한 줄로 파일 끝에 붙인다. best-effort(실패는 무시).
func (r *Recorder) append(rec Record) {
	rec.Ts = r.now().Format(time.RFC3339)
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
```

> 주의: best-effort라 `append`는 에러를 반환하지 않는다. 호출 측 로깅이 필요하면 후속에서 추가(현재 YAGNI). `TestRecorder_BestEffortNoPanic`이 실패 경로 무중단을 보장한다.

- [ ] **Step 4: 구현 (context.go)**

`internal/usage/context.go`:
```go
package usage

import "context"

type featureKey struct{}

// WithFeature 는 ctx 에 비용 기록용 feature 라벨을 심는다.
func WithFeature(ctx context.Context, feature string) context.Context {
	return context.WithValue(ctx, featureKey{}, feature)
}

// FeatureFromContext 는 ctx 의 feature 라벨을 꺼낸다(없으면 "").
func FeatureFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(featureKey{}).(string); ok {
		return v
	}
	return ""
}
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -race -v`
Expected: PASS (race 포함).

- [ ] **Step 6: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/usage/recorder.go internal/usage/context.go internal/usage/recorder_test.go
git commit -m "feat(usage): JSONL Recorder(append, best-effort, 동시성) + feature context"
```

---

### Task 3: Query/Summary 집계 + 기간 계산

**Files:**
- Create: `internal/usage/query.go`
- Test: `internal/usage/query_test.go`

**Interfaces:**
- Consumes: `Record`, `Recorder` (Task 2).
- Produces:
  - `type Bucket struct { Key string; Calls, InputTk, OutputTk int; CostUSD float64 }`
  - `type Summary struct { From, To time.Time; TotalCost float64; TotalCalls int; ByModel, ByFeature []Bucket }`
  - `func (r *Recorder) Query(from, to time.Time) (Summary, error)` — `[from, to)` 반열린 구간.
  - `func RangeForPeriod(now time.Time, period string) (from, to time.Time)` — `today`/`week`/`month`, 그 외 today. `to` = now 시점 약간 이후(미래 포함 위해 now 사용), `from` = 구간 시작. **반환은 now 의 Location 기준.**
- ByModel/ByFeature 정렬: CostUSD 내림차순(동률이면 Key 오름차순) — 결정적 출력.

- [ ] **Step 1: 실패 테스트 작성**

`internal/usage/query_test.go`:
```go
package usage

import (
	"testing"
	"time"
)

// seed 는 고정 ts 로 레코드를 직접 기록한다(now 고정).
func seedAt(r *Recorder, ts time.Time, log func()) {
	r.now = func() time.Time { return ts }
	log()
}

func TestQuery_FilterAndAggregate(t *testing.T) {
	t.Parallel()
	r, _ := fixedRecorder(t)
	base := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	seedAt(r, base, func() { r.LogGemini("agent", "gemini-2.5-flash", 1000, 100) })          // 0.0003 + 0.00025 = 0.00055
	seedAt(r, base.Add(time.Hour), func() { r.LogGemini("vision", "gemini-2.5-flash-lite", 500, 50) }) // 0.00005 + 0.00002 = 0.00007
	seedAt(r, base.Add(2*time.Hour), func() { r.LogClaude("kb", "claude", 100, 10, 0.12) })
	seedAt(r, base.AddDate(0, 0, -1), func() { r.LogGemini("agent", "gemini-2.5-flash", 999, 999) }) // 어제 → 제외

	from := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	s, err := r.Query(from, to)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.TotalCalls != 3 {
		t.Fatalf("TotalCalls = %d, want 3", s.TotalCalls)
	}
	wantCost := 0.00055 + 0.00007 + 0.12
	if d := s.TotalCost - wantCost; d < -1e-9 || d > 1e-9 {
		t.Fatalf("TotalCost = %v, want %v", s.TotalCost, wantCost)
	}
	if len(s.ByFeature) != 3 {
		t.Fatalf("ByFeature buckets = %d, want 3", len(s.ByFeature))
	}
	// 정렬: claude(kb, 0.12) 가 최상위
	if s.ByFeature[0].Key != "kb" {
		t.Fatalf("top feature = %q, want kb", s.ByFeature[0].Key)
	}
}

func TestQuery_MissingFileIsEmpty(t *testing.T) {
	t.Parallel()
	r, _ := fixedRecorder(t) // 아직 아무것도 기록 안 함 → 파일 없음
	s, err := r.Query(time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("missing file should be empty, got err %v", err)
	}
	if s.TotalCalls != 0 {
		t.Fatalf("want 0 calls, got %d", s.TotalCalls)
	}
}

func TestRangeForPeriod(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 14, 30, 0, 0, time.UTC) // 월요일
	tests := []struct{ period string; wantFrom time.Time }{
		{"today", time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)},
		{"month", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"", time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)}, // 기본 today
	}
	for _, tt := range tests {
		from, to := RangeForPeriod(now, tt.period)
		if !from.Equal(tt.wantFrom) {
			t.Errorf("period %q: from = %v, want %v", tt.period, from, tt.wantFrom)
		}
		if !to.After(now) && !to.Equal(now) {
			t.Errorf("period %q: to %v should be >= now %v", tt.period, to, now)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -run 'TestQuery|TestRangeForPeriod' -v`
Expected: FAIL (`undefined: Query` 등).

- [ ] **Step 3: 구현**

`internal/usage/query.go`:
```go
package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"time"
)

// Bucket 은 한 키(model 또는 feature)의 집계다.
type Bucket struct {
	Key      string
	Calls    int
	InputTk  int
	OutputTk int
	CostUSD  float64
}

// Summary 는 기간 집계 결과다.
type Summary struct {
	From, To   time.Time
	TotalCost  float64
	TotalCalls int
	ByModel    []Bucket
	ByFeature  []Bucket
}

// Query 는 [from, to) 구간의 레코드를 읽어 집계한다. 파일 없으면 빈 Summary.
func (r *Recorder) Query(from, to time.Time) (Summary, error) {
	s := Summary{From: from, To: to}
	r.mu.Lock()
	f, err := os.Open(r.path)
	r.mu.Unlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	defer f.Close()

	models := map[string]*Bucket{}
	feats := map[string]*Bucket{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue // 깨진 줄은 건너뜀(best-effort)
		}
		ts, err := time.Parse(time.RFC3339, rec.Ts)
		if err != nil {
			continue
		}
		if ts.Before(from) || !ts.Before(to) { // [from, to)
			continue
		}
		s.TotalCalls++
		s.TotalCost += rec.CostUSD
		addBucket(models, rec.Model, rec)
		addBucket(feats, rec.Feature, rec)
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	s.ByModel = sortedBuckets(models)
	s.ByFeature = sortedBuckets(feats)
	return s, nil
}

func addBucket(m map[string]*Bucket, key string, rec Record) {
	b := m[key]
	if b == nil {
		b = &Bucket{Key: key}
		m[key] = b
	}
	b.Calls++
	b.InputTk += rec.InputTk
	b.OutputTk += rec.OutputTk
	b.CostUSD += rec.CostUSD
}

func sortedBuckets(m map[string]*Bucket) []Bucket {
	out := make([]Bucket, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// RangeForPeriod 는 period("today"|"week"|"month")에 대한 [from, to) 를 now 기준으로 계산한다.
// to 는 now(미래 레코드까지 포함하도록 넉넉히), from 은 구간 시작 00:00.
func RangeForPeriod(now time.Time, period string) (from, to time.Time) {
	loc := now.Location()
	y, mo, d := now.Date()
	startOfDay := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch period {
	case "week":
		// 월요일 시작
		offset := (int(now.Weekday()) + 6) % 7
		from = startOfDay.AddDate(0, 0, -offset)
	case "month":
		from = time.Date(y, mo, 1, 0, 0, 0, 0, loc)
	default: // today
		from = startOfDay
	}
	return from, now
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/usage/ -v`
Expected: PASS (전체 usage 패키지).

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/usage/query.go internal/usage/query_test.go
git commit -m "feat(usage): 기간 집계 Query/Summary + RangeForPeriod"
```

---

### Task 4: gemini sink 주입 + 자동 기록

**Files:**
- Modify: `internal/gemini/client.go` (UsageSink 인터페이스, sink 필드, SetUsageSink, record 헬퍼)
- Modify: `internal/gemini/generate.go` (GenerateText/GenerateWithTools 에 record 호출)
- Modify: `internal/gemini/vision.go` (ExtractItems 에 record 호출)
- Test: `internal/gemini/usage_test.go`

**Interfaces:**
- Consumes: `usage.FeatureFromContext` (Task 2).
- Produces:
  - `type UsageSink interface { LogGemini(feature, model string, inputTk, outputTk int) }`
  - `func (c *Client) SetUsageSink(s UsageSink)`
  - 내부: `func (c *Client) record(ctx context.Context, resp *genai.GenerateContentResponse, defaultFeature string)`.

- [ ] **Step 1: 실패 테스트 작성**

`internal/gemini/usage_test.go`:
```go
package gemini

import (
	"context"
	"testing"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

type fakeSink struct {
	feature, model        string
	inputTk, outputTk     int
	calls                 int
}

func (f *fakeSink) LogGemini(feature, model string, inputTk, outputTk int) {
	f.calls++
	f.feature, f.model, f.inputTk, f.outputTk = feature, model, inputTk, outputTk
}

func TestRecord_UsesDefaultFeature(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash")
	fs := &fakeSink{}
	c.SetUsageSink(fs)
	resp := &genai.GenerateContentResponse{
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 1200, CandidatesTokenCount: 80,
		},
	}
	c.record(context.Background(), resp, "agent")
	if fs.calls != 1 || fs.feature != "agent" || fs.model != "gemini-2.5-flash" || fs.inputTk != 1200 || fs.outputTk != 80 {
		t.Fatalf("unexpected sink state: %+v", fs)
	}
}

func TestRecord_ContextOverridesFeature(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash")
	fs := &fakeSink{}
	c.SetUsageSink(fs)
	resp := &genai.GenerateContentResponse{
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 1},
	}
	ctx := usage.WithFeature(context.Background(), "knowledge")
	c.record(ctx, resp, "text")
	if fs.feature != "knowledge" {
		t.Fatalf("feature = %q, want knowledge (ctx override)", fs.feature)
	}
}

func TestRecord_NilSafe(t *testing.T) {
	t.Parallel()
	c := New("k", "gemini-2.5-flash") // sink 미설정
	c.record(context.Background(), nil, "agent")          // resp nil
	c.record(context.Background(), &genai.GenerateContentResponse{}, "agent") // UsageMetadata nil
	c.SetUsageSink(&fakeSink{})
	c.record(context.Background(), &genai.GenerateContentResponse{}, "agent") // metadata nil + sink 있음 → 호출 안 함
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/gemini/ -run TestRecord -v`
Expected: FAIL (`c.record undefined`, `SetUsageSink undefined`).

- [ ] **Step 3: client.go 수정**

`internal/gemini/client.go` 의 import 에 추가:
```go
	"context"

	"github.com/Jongseong0111/jarvis/internal/usage"
```
(기존 import 들과 병합. `context` 는 이미 다른 파일에서 쓰지만 이 파일에도 필요.)

`Client` 구조체에 필드 추가:
```go
type Client struct {
	apiKey string
	model  string
	sink   UsageSink
}
```

파일 끝에 추가:
```go
// UsageSink 는 Gemini 호출 비용을 기록하는 대상이다(usage.Recorder 가 구현).
type UsageSink interface {
	LogGemini(feature, model string, inputTk, outputTk int)
}

// SetUsageSink 는 비용 기록 sink 를 주입한다(nil 이면 기록 안 함).
func (c *Client) SetUsageSink(s UsageSink) { c.sink = s }

// record 는 응답의 토큰 사용량을 sink 에 기록한다. feature 는 ctx 값 우선, 없으면 defaultFeature.
func (c *Client) record(ctx context.Context, resp *genai.GenerateContentResponse, defaultFeature string) {
	if c.sink == nil || resp == nil || resp.UsageMetadata == nil {
		return
	}
	feature := usage.FeatureFromContext(ctx)
	if feature == "" {
		feature = defaultFeature
	}
	m := resp.UsageMetadata
	c.sink.LogGemini(feature, c.model, int(m.PromptTokenCount), int(m.CandidatesTokenCount))
}
```

- [ ] **Step 4: 호출 지점에 record 추가**

`internal/gemini/generate.go` `GenerateText` 의 `resp, err := ...GenerateContent(...)` 직후, 에러 처리 다음 줄에 추가:
```go
	c.record(ctx, resp, "text")
```
(위치: `resp, err :=` 의 err nil 통과 후, `out := strings.TrimSpace(...)` 전.)

`internal/gemini/generate.go` 에는 `GenerateWithTools` 가 없다(client.go 에 있음). `internal/gemini/client.go` `GenerateWithTools` 의 `return resp, nil` 직전에 추가:
```go
	c.record(ctx, resp, "agent")
	return resp, nil
```

`internal/gemini/vision.go` `ExtractItems` 의 `resp, err := ...GenerateContent(...)` 에러 처리 직후에 추가:
```go
	c.record(ctx, resp, "vision")
```

- [ ] **Step 5: 테스트 통과 + 빌드 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/gemini/ -run TestRecord -v && go build ./...`
Expected: PASS + 빌드 성공.

- [ ] **Step 6: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/gemini/
git commit -m "feat(gemini): UsageSink 주입 + 모든 호출 비용 자동 기록(feature ctx)"
```

---

### Task 5: claudecode sink 주입 + 비용 파싱

**Files:**
- Modify: `internal/claudecode/runner.go`
- Test: `internal/claudecode/runner_test.go` (기존 있으면 추가, 없으면 생성)

**Interfaces:**
- Produces:
  - `RunResult` 에 필드 추가: `InputTk, OutputTk int; CostUSD float64`.
  - `type UsageSink interface { LogClaude(feature, model string, inputTk, outputTk int, costUSD float64) }`
  - `func (r *CLIRunner) SetUsageSink(s UsageSink)`
  - `ParseOutput` 가 cost/token 채움(model 없으면 `"claude"`).

- [ ] **Step 1: 실패 테스트 작성**

`internal/claudecode/runner_test.go` 에 추가(파일 없으면 생성, `package claudecode`):
```go
package claudecode

import "testing"

func TestParseOutput_Cost(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"result","is_error":false,"result":"Hi","session_id":"s1","total_cost_usd":0.116938,"usage":{"input_tokens":5880,"output_tokens":5}}`)
	got, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if got.SessionID != "s1" || got.Text != "Hi" {
		t.Fatalf("base fields wrong: %+v", got)
	}
	if got.InputTk != 5880 || got.OutputTk != 5 || got.CostUSD != 0.116938 {
		t.Fatalf("cost fields wrong: %+v", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/claudecode/ -run TestParseOutput_Cost -v`
Expected: FAIL (`got.InputTk undefined`).

- [ ] **Step 3: 구현**

`internal/claudecode/runner.go`:

`RunResult` 에 필드 추가:
```go
type RunResult struct {
	SessionID string
	Text      string
	InputTk   int
	OutputTk  int
	CostUSD   float64
}
```

`cliOutput` 확장:
```go
type cliOutput struct {
	SessionID string  `json:"session_id"`
	Result    string  `json:"result"`
	IsError   bool    `json:"is_error"`
	TotalCost float64 `json:"total_cost_usd"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
```

`ParseOutput` 의 성공 반환 수정:
```go
	return RunResult{
		SessionID: o.SessionID,
		Text:      o.Result,
		InputTk:   o.Usage.InputTokens,
		OutputTk:  o.Usage.OutputTokens,
		CostUSD:   o.TotalCost,
	}, nil
```

`CLIRunner` 에 sink 추가:
```go
type CLIRunner struct {
	sink UsageSink
}

// UsageSink 는 Claude 호출 비용 기록 대상이다(usage.Recorder 가 구현).
type UsageSink interface {
	LogClaude(feature, model string, inputTk, outputTk int, costUSD float64)
}

// SetUsageSink 는 비용 기록 sink 를 주입한다(nil 이면 기록 안 함).
func (r *CLIRunner) SetUsageSink(s UsageSink) { r.sink = s }
```

`exec` 의 `return ParseOutput(out)` 를 교체:
```go
	res, err := ParseOutput(out)
	if err != nil {
		return RunResult{}, err
	}
	if r.sink != nil {
		r.sink.LogClaude("kb", "claude", res.InputTk, res.OutputTk, res.CostUSD)
	}
	return res, nil
```

> `New()` 는 `&CLIRunner{}` 그대로(sink nil 기본). 시그니처 불변.

- [ ] **Step 4: 테스트 통과 + 빌드 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/claudecode/ -v && go build ./...`
Expected: PASS + 빌드 성공.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/claudecode/
git commit -m "feat(claudecode): total_cost_usd/usage 파싱 + UsageSink 주입(kb)"
```

---

### Task 6: config UsageLogPath

**Files:**
- Modify: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `Config.UsageLogPath string`, 기본 `~/.jarvis/usage.jsonl`(`expandHome` 적용). 필수 아님.

- [ ] **Step 1: 실패 테스트 작성**

`pkg/config/config_test.go` 에 추가:
```go
func TestUsageLogPathDefault(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "x")
	t.Setenv("SLACK_APP_TOKEN", "x")
	t.Setenv("GEMINI_API_KEY", "x")
	t.Setenv("NOTION_API_KEY", "x")
	t.Setenv("NOTION_LOCATIONS_DB_ID", "x")
	t.Setenv("NOTION_CATEGORIES_DB_ID", "x")
	t.Setenv("NOTION_ITEMS_DB_ID", "x")
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.HasSuffix(cfg.UsageLogPath, "/.jarvis/usage.jsonl") {
		t.Fatalf("UsageLogPath = %q, want ~/.jarvis/usage.jsonl", cfg.UsageLogPath)
	}
}
```
> 기존 `config_test.go` 의 import 에 `strings` 가 없으면 추가. 필수 env 키 목록은 기존 통과 테스트에서 복사해 정확히 맞출 것(validate 통과 위해).

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/ -run TestUsageLogPathDefault -v`
Expected: FAIL (`cfg.UsageLogPath undefined`).

- [ ] **Step 3: 구현**

`pkg/config/config.go` `Config` 구조체에 추가(예: `DigestRSSURLs` 아래):
```go
	UsageLogPath string // LLM 비용 기록 JSONL 경로
```

`New()` 의 `cfg := Config{...}` 안에 추가:
```go
		UsageLogPath: expandHome(getenv("USAGE_LOG_PATH", "~/.jarvis/usage.jsonl")),
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add pkg/config/
git commit -m "feat(config): USAGE_LOG_PATH(기본 ~/.jarvis/usage.jsonl)"
```

---

### Task 7: list_usage 도구

**Files:**
- Create: `internal/agent/usage_tools.go`
- Test: `internal/agent/usage_tools_test.go`

**Interfaces:**
- Consumes: `usage.Summary`, `usage.RangeForPeriod`, `usage.Recorder` (Tasks 2-3); `Tool`, `strArg`, `objSchema`, `strSchema` (기존 `internal/agent/tools.go`).
- Produces:
  - `type UsageReader interface { Query(from, to time.Time) (usage.Summary, error) }`
  - `func UsageTools(r UsageReader) []Tool` — `list_usage` 도구 1개(읽기, `Run`).
  - 내부: `func formatUsage(period string, s usage.Summary) string`.

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/usage_tools_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

type fakeUsageReader struct{ s usage.Summary }

func (f fakeUsageReader) Query(from, to time.Time) (usage.Summary, error) { return f.s, nil }

func TestListUsageTool(t *testing.T) {
	t.Parallel()
	s := usage.Summary{
		TotalCost: 0.0123, TotalCalls: 47,
		ByModel: []usage.Bucket{
			{Key: "gemini-2.5-flash", Calls: 31, CostUSD: 0.0098},
			{Key: "claude", Calls: 4, CostUSD: 0.0014},
		},
		ByFeature: []usage.Bucket{
			{Key: "agent", Calls: 31, CostUSD: 0.0090},
			{Key: "kb", Calls: 4, CostUSD: 0.0014},
		},
	}
	tools := UsageTools(fakeUsageReader{s: s})
	if len(tools) != 1 || tools[0].Decl.Name != "list_usage" {
		t.Fatalf("want 1 list_usage tool, got %+v", tools)
	}
	if tools[0].Write {
		t.Fatal("list_usage must be read-only")
	}
	out, err := tools[0].Run(context.Background(), map[string]any{"period": "today"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"오늘", "$0.0123", "47", "gemini-2.5-flash", "agent", "•"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestListUsageTool -v`
Expected: FAIL (`undefined: UsageTools`).

- [ ] **Step 3: 구현**

`internal/agent/usage_tools.go`:
```go
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

// UsageReader 는 비용 집계를 읽는 능력이다(usage.Recorder 가 구현).
type UsageReader interface {
	Query(from, to time.Time) (usage.Summary, error)
}

// UsageTools 는 비용 조회 도구(list_usage)를 만든다.
func UsageTools(r UsageReader) []Tool {
	return []Tool{{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_usage",
			Description: "기간별 LLM 사용 비용을 조회한다(오늘/이번주/이번달). 사용자가 비용·요금을 물으면 사용.",
			Parameters: objSchema(map[string]*genai.Schema{
				"period": strSchema("기간: today(기본), week, month 중 하나"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			period := strArg(args, "period")
			if period == "" {
				period = "today"
			}
			from, to := usage.RangeForPeriod(time.Now(), period)
			s, err := r.Query(from, to)
			if err != nil {
				return "", fmt.Errorf("비용 조회 실패: %w", err)
			}
			return formatUsage(period, s), nil
		},
	}}
}

func periodLabel(period string) string {
	switch period {
	case "week":
		return "이번주"
	case "month":
		return "이번달"
	default:
		return "오늘"
	}
}

// formatUsage 는 집계를 존댓말+이모지+• 불릿으로 포맷한다.
func formatUsage(period string, s usage.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💰 %s LLM 비용: $%.4f (%d회 호출)\n", periodLabel(period), s.TotalCost, s.TotalCalls)
	if s.TotalCalls == 0 {
		return b.String() + "\n아직 기록된 호출이 없습니다."
	}
	b.WriteString("\n*모델별*\n")
	for _, m := range s.ByModel {
		fmt.Fprintf(&b, "• %s: $%.4f (%d회)\n", m.Key, m.CostUSD, m.Calls)
	}
	b.WriteString("\n*기능별*\n")
	for _, f := range s.ByFeature {
		fmt.Fprintf(&b, "• %s: $%.4f (%d회)\n", f.Key, f.CostUSD, f.Calls)
	}
	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestListUsageTool -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/agent/usage_tools.go internal/agent/usage_tools_test.go
git commit -m "feat(agent): list_usage 도구(기간별 비용 조회, 모델/기능별)"
```

---

### Task 8: main 배선 + 호출자 ctx 태깅 + 최종 검증

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/knowledge/service.go` (GenerateText 호출 전 ctx 태깅)
- Modify: `internal/devdigest/digest.go` (GenerateText 호출 전 ctx 태깅, 2곳)

**Interfaces:**
- Consumes: `usage.NewRecorder` (Task 2), `usage.WithFeature` (Task 2), `gemini.Client.SetUsageSink` (Task 4), `claudecode.CLIRunner.SetUsageSink` (Task 5), `agent.UsageTools` (Task 7), `Config.UsageLogPath` (Task 6).

- [ ] **Step 1: main.go 배선**

`cmd/server/main.go` import 에 추가:
```go
	"github.com/Jongseong0111/jarvis/internal/usage"
```

`geminiClient`/`visionClient`/`ccRunner` 생성 이후(셋 다 만들어진 뒤, 예: `ccRunner := claudecode.New()` 다음)에 추가:
```go
	rec := usage.NewRecorder(cfg.UsageLogPath)
	geminiClient.SetUsageSink(rec)
	visionClient.SetUsageSink(rec)
	ccRunner.SetUsageSink(rec)
```

도구 등록부(다른 `tools = append(...)` 들과 같은 위치, `ag := agent.New(...)` 전)에 추가:
```go
	tools = append(tools, agent.UsageTools(rec)...)
```

- [ ] **Step 2: knowledge 서비스 ctx 태깅**

`internal/knowledge/service.go` import 에 `"github.com/Jongseong0111/jarvis/internal/usage"` 추가. `s.sum.GenerateText(ctx, summarySystem, ...)` 호출 직전에:
```go
	ctx = usage.WithFeature(ctx, "knowledge")
```
(같은 함수 안, GenerateText 호출 전. ctx 가 파라미터로 들어오면 재할당, 없으면 함수 시그니처 확인 후 적절히.)

- [ ] **Step 3: devdigest ctx 태깅**

`internal/devdigest/digest.go` import 에 `usage` 추가. 두 GenerateText 호출(`digest.go:62`, `digest.go:77`) 각각 직전에:
```go
	ctx = usage.WithFeature(ctx, "digest")
```
(각 메서드의 ctx 변수에 맞춰. ctx 가 없으면 메서드 시그니처 확인.)

- [ ] **Step 4: 빌드 + 전체 테스트 + vet**

Run:
```bash
cd ~/personal-agent/jarvis && go build ./... && go vet ./... && go test ./... -race
```
Expected: 전부 성공(기존 테스트 포함 green).

- [ ] **Step 5: 라이브 스모크 검증(수동)**

```bash
cd ~/personal-agent/jarvis && go build -o bin/jarvis ./cmd/server && \
  nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
```
슬랙에서:
1. 아무 메시지(예: "안녕") 보내 에이전트 호출 발생 → `~/.jarvis/usage.jsonl` 에 줄 생기는지 확인:
   `tail -2 ~/.jarvis/usage.jsonl` → `source:"gemini", feature:"agent"` 줄 존재.
2. "오늘 비용 얼마야?" → `list_usage` 호출 → 모델별/기능별 집계 응답 확인.
3. 서버 재시작(`pkill -f bin/jarvis` 후 재실행) → "오늘 비용" 다시 조회 → **이전 기록이 유지**되는지 확인(영속성).

- [ ] **Step 6: 커밋**

```bash
cd ~/personal-agent/jarvis
git add cmd/server/main.go internal/knowledge/service.go internal/devdigest/digest.go
git commit -m "feat(server): usage Recorder 배선 + knowledge/digest feature 태깅"
```

---

## 자체 리뷰 결과(작성자 체크)

- **Spec 커버리지:** §2 결정(JSONL/영속/단위/차원/주입/토글) → Tasks 1-8 전부 매핑. §3 데이터 모델 → Task 2 Record. §4.1 Recorder → Task 2-3. §4.2 가격표 → Task 1. §4.3 sink → Task 4-5. §4.4 feature ctx → Task 2(정의)+4(소비)+8(태깅). §4.5 list_usage → Task 7. §4.6 배선 → Task 8. §4.7 config → Task 6. §5 Claude 파싱 → Task 5. §7 테스트 → 각 Task TDD. 갭 없음.
- **플레이스홀더:** 가격 수치 실측 반영(0.30/2.50, 0.10/0.40), Claude 필드 실측 반영. TBD 없음.
- **타입 일관성:** `LogGemini(feature, model, inputTk, outputTk)` / `LogClaude(..., costUSD)` / `Query(from, to) (Summary, error)` / `RangeForPeriod(now, period) (from, to)` / `Bucket.Key` — Task 정의와 소비처 일치 확인.

## 범위 밖 (YAGNI)
파일 로테이션, 명시 날짜 범위 질의, 예산 알림, 대화 루프 내 도구별 세분화.
