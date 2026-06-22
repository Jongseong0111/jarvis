# jarvis LLM 비용 추적 설계

- 작성일: 2026-06-22
- 상태: 설계 승인 완료, 구현 대기
- 브랜치: `feature/llm-cost-tracking`

## 1. 목적

jarvis가 호출하는 모든 LLM(Gemini, Claude Code) 비용을 로컬에 영구 기록해, "오늘 비용 얼마야?" 같은 자연어 질의를 슬랙에서 처리한다.

**Why:** LLM 비용이 여러 모델·기능에 분산돼 있어 파악이 어렵다. 어떤 기능이 돈을 많이 쓰는지 보고 싶다.

## 2. 핵심 결정

| 항목 | 결정 |
|------|------|
| 저장소 | `~/.jarvis/usage.jsonl` (JSONL, append-only, stdlib만, 의존성 0) |
| 영속성 | **디스크 파일** — 서버 재시작/리빌드/`pkill`과 무관하게 누적 유지. 인메모리 아님. |
| 기록 단위 | **API 호출 1번 = 1줄**(에이전트 함수콜 루프는 호출마다 기록 → 정확한 과금 반영) |
| 집계 차원 | 총액 + `source`/`model`별 + `feature`별 |
| Gemini 비용 | 가격표(코드 하드코딩) × `UsageMetadata` 토큰 |
| Claude 비용 | runner JSON의 `total_cost_usd` 직접 사용(가격표 불필요) |
| 주입 방식 | `gemini.Client`/`claudecode.CLIRunner`에 sink setter 주입 → 호출 지점 수정 0 |
| 기능 토글 | 항상 켜짐(가벼움). 경로만 `USAGE_LOG_PATH`로 설정 가능 |

## 3. 데이터 모델

`~/.jarvis/usage.jsonl`, 1줄 = 1 API 호출:

```json
{"ts":"2026-06-22T14:03:11+09:00","source":"gemini","feature":"agent","model":"gemini-2.5-flash","input_tk":1820,"output_tk":95,"cost_usd":0.000612}
```

- `ts`: RFC3339 (로컬 TZ)
- `source`: `gemini` | `claude`
- `feature`: `agent` | `vision` | `knowledge` | `digest` | `kb`
- `input_tk` / `output_tk`: 정수 토큰 수
- `cost_usd`: 계산된 USD 비용(float)

### feature 라벨 정의

| feature | 출처 호출 |
|---------|-----------|
| `agent` | 메인 대화 루프 `GenerateWithTools` (옴니 에이전트 — 도구별 세분화 안 함) |
| `vision` | `ExtractItems` (사진 물건 인식) |
| `knowledge` | `GenerateText` (ChatGPT 공유 요약) |
| `digest` | `GenerateText` (dev digest / 공부 주제) |
| `kb` | Claude Code ingest (`claudecode` runner) |

## 4. 컴포넌트

### 4.1 `internal/usage` (신규 패키지, stdlib만)

```go
// Record 는 usage.jsonl 한 줄이다.
type Record struct {
    Ts       string  `json:"ts"`
    Source   string  `json:"source"`
    Feature  string  `json:"feature"`
    Model    string  `json:"model"`
    InputTk  int     `json:"input_tk"`
    OutputTk int     `json:"output_tk"`
    CostUSD  float64 `json:"cost_usd"`
}

// Recorder 는 비용 기록(append)과 조회(집계)를 담당한다.
type Recorder struct {
    path string
    mu   sync.Mutex
    now  func() time.Time // 테스트 정적 시간 주입
}

func NewRecorder(path string) *Recorder

// LogGemini 는 Gemini 호출 1건을 기록한다(가격표로 cost 계산).
func (r *Recorder) LogGemini(feature, model string, inputTk, outputTk int)

// LogClaude 는 Claude 호출 1건을 기록한다(cost 외부 제공).
func (r *Recorder) LogClaude(feature, model string, inputTk, outputTk int, costUSD float64)

// Query 는 기간 내 레코드를 읽어 집계한다.
func (r *Recorder) Query(from, to time.Time) (Summary, error)
```

- append: `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0o600)` → JSON 한 줄 → mutex로 직렬화. 디렉터리 없으면 `os.MkdirAll`.
- 기록 실패는 best-effort: slog 경고만, 패닉/에러 전파 없음(사용자 요청 절대 안 막음).
- `Query`: 파일 줄 단위 스캔 → `ts` 파싱해 `[from,to)` 필터 → 총합 + `bySource`/`byModel`/`byFeature` 맵 집계.

```go
type Summary struct {
    From, To   time.Time
    TotalCost  float64
    TotalCalls int
    ByModel    []Bucket // model, calls, inputTk, outputTk, cost
    ByFeature  []Bucket
}
```

### 4.2 가격표 (`internal/usage/pricing.go`)

```go
type price struct{ inPer1M, outPer1M float64 } // USD per 1M tokens
var geminiPrices = map[string]price{
    "gemini-2.5-flash":      {/* 구현 시 현재 공시가 */},
    "gemini-2.5-flash-lite": {/* 구현 시 현재 공시가 */},
}
func geminiCost(model string, inTk, outTk int) float64 // 모르는 모델 → 0(토큰은 기록)
```

> 가격 수치는 구현 시 Google 공시 가격으로 채운다. 미지 모델은 cost=0으로 기록하되 토큰은 남겨 추후 보정 가능.

### 4.3 sink 주입 (import cycle 회피)

`gemini`와 `claudecode`는 각자 작은 sink 인터페이스를 **로컬 정의**하고 setter로 주입받는다. `usage.Recorder`가 둘 다 만족한다.

```go
// gemini 패키지
type UsageSink interface {
    LogGemini(feature, model string, inputTk, outputTk int)
}
func (c *Client) SetUsageSink(s UsageSink) // nil 허용(no-op)

// claudecode 패키지
type UsageSink interface {
    LogClaude(feature, model string, inputTk, outputTk int, costUSD float64)
}
func (r *CLIRunner) SetUsageSink(s UsageSink)
```

기존 호출자(server×2, sendbrief×1)는 setter 호출 안 하면 nil → 기록 생략. **메서드 시그니처 불변.**

### 4.4 feature 라벨 전달 (`ctx` 기반)

```go
// usage 패키지
func WithFeature(ctx context.Context, feature string) context.Context
func FeatureFromContext(ctx context.Context) string // 없으면 ""
```

- `gemini.Client`의 각 메서드는 응답 후 `record(ctx, resp, defaultFeature)` 호출.
  - `record`는 `usage.FeatureFromContext(ctx)`가 있으면 그것, 없으면 메서드 기본값 사용.
  - `GenerateWithTools` 기본 `agent`, `ExtractItems` 기본 `vision`, `GenerateText` 기본 `text`.
- `GenerateText` 호출자가 ctx 태깅:
  - `knowledge.Service` → `usage.WithFeature(ctx, "knowledge")`
  - `devdigest.Generator` → `usage.WithFeature(ctx, "digest")`
- `claudecode` runner는 `kb` 고정(현재 유일 용도).

> 주의: `gemini`/`claudecode`가 `usage.FeatureFromContext`를 쓰면 `usage`를 import한다. `usage`는 이들을 import하지 않으므로 cycle 없음. (sink 인터페이스는 여전히 로컬 정의해 결합 최소화.)

### 4.5 `internal/agent/usage_tools.go` — `list_usage` 도구

- 읽기 전용(`Run`, 즉시 실행), 버튼 승인 없음.
- 인자: `period` (`today`|`week`|`month`, 기본 `today`). (명시 `from`/`to`는 YAGNI — 후속.)
- 동작: period → `[from,to)` 계산 → `Recorder.Query` → 존댓말 + 이모지 + `•` 불릿 포맷.
- 출력 예:
  ```
  💰 오늘 LLM 비용: $0.0123 (47회 호출)

  모델별
  • gemini-2.5-flash: $0.0098 (31회)
  • gemini-2.5-flash-lite: $0.0011 (12회)
  • claude: $0.0014 (4회)

  기능별
  • agent: $0.0090
  • vision: $0.0011
  • kb: $0.0014
  • digest: $0.0008
  ```

### 4.6 배선 (`cmd/server/main.go`)

```go
rec := usage.NewRecorder(cfg.UsageLogPath)
geminiClient.SetUsageSink(rec)
visionClient.SetUsageSink(rec)
ccRunner.SetUsageSink(rec)
tools = append(tools, agent.UsageTools(rec)...)
```

`cmd/sendbrief`는 비용 기록 불필요 → setter 미호출(또는 동일 주입, 선택).

### 4.7 설정 (`pkg/config/config.go`)

- 추가: `UsageLogPath string`, 기본 `~/.jarvis/usage.jsonl` (`expandHome`).
- 필수값 아님(validate 변경 없음).

## 5. Claude 출력 파싱 확장

`claudecode.cliOutput`에 비용/토큰 필드 추가:

```go
type cliOutput struct {
    SessionID  string `json:"session_id"`
    Result     string `json:"result"`
    IsError    bool   `json:"is_error"`
    TotalCost  float64 `json:"total_cost_usd"`
    Usage      struct {
        InputTokens  int `json:"input_tokens"`
        OutputTokens int `json:"output_tokens"`
    } `json:"usage"`
    Model string `json:"model"` // 있으면 사용, 없으면 "claude"
}
```

`exec`가 `ParseOutput` 후 sink 있으면 `LogClaude("kb", model, in, out, cost)` 호출. 필드 부재(0값)면 cost 0으로 기록(무해).

## 6. 안전 / 운영

- 기록은 best-effort, 실패해도 사용자 요청 흐름 차단 없음.
- 동시 쓰기(에이전트 루프 + 스케줄러 브리핑 goroutine)는 `sync.Mutex`로 직렬화.
- 용량: 1줄 ~150B → 연간 한 자릿수 MB. 로테이션 YAGNI(후속 시 월별 파일 분리 용이).

## 7. 테스트 전략

- `Recorder`: temp dir append→`Query` round-trip / 동시 append `-race` / 기간 필터 경계(`[from,to)`) / best-effort(쓰기 불가 경로에서 패닉 없음).
- `pricing`: table-driven cost 계산, 미지 모델 0.
- sink 주입: gemini/claudecode에 fake sink로 호출·인자 검증(특히 feature 기본값/ctx 오버라이드).
- `list_usage`: 고정 JSONL fixture → 집계·포맷 골든 검증.
- 정적 시간(`now` 주입), `t.Parallel()`.

## 8. 범위 밖 (YAGNI)

- 파일 로테이션/압축
- 명시 날짜 범위 질의(`from`/`to` 자유 입력)
- 예산 알림("이번 달 $N 초과 시 알림")
- 대화 루프 내 도구별 세분화
