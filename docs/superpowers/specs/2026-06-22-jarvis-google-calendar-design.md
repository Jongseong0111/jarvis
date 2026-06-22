# jarvis Google Calendar 연동 설계

- 작성일: 2026-06-22
- 상태: 설계 승인 완료, 구현 대기
- 브랜치: `feature/google-calendar`

## 1. 목적

Google Calendar를 jarvis에 연동해 슬랙 자연어로 일정을 조회/추가/삭제/검색하고, 아침 브리핑에 오늘 일정을 포함한다.

**Why:** 일정 조회·추가·삭제·검색을 슬랙 한 곳에서 처리하고, 할일과 합쳐 "급한 일"을 한 번에 보고 싶다.

**예시 입력:**
- "이번주 일정 알려줘", "급하게 해야 하는 일 알려줘"
- "다음주 월요일 3시에 미팅 추가해줘", "아기 검진 7월 3일에 추가해줘"
- "아기 검진 언제였지?"
- "그 미팅 삭제해줘"

## 2. 핵심 결정

| 항목 | 결정 |
|------|------|
| API 클라이언트 | **공식 SDK** `google.golang.org/api/calendar/v3` + `golang.org/x/oauth2/google` (OAuth2 토큰 자동 갱신) |
| 인증 | OAuth2 Desktop app, refresh token을 `.env`에 저장(1회 브라우저 로그인) |
| add_event | **즉시 실행**(Todoist add 패턴) |
| delete_event | **버튼 승인**(안전 원칙: 삭제는 항상 승인) |
| 상대 날짜 | 시스템 프롬프트에 현재 날짜/시각(Asia/Seoul) 주입 → LLM이 RFC3339로 변환 |
| "급한 일" | 별도 도구 없음 — 에이전트가 list_events + list_todos 합성 |
| 브리핑 | 아침 브리핑에 오늘 일정 섹션 추가(저녁은 범위 밖) |
| 대상 캘린더 | `GOOGLE_CALENDAR_ID`(기본 `primary`) |
| 기능 토글 | refresh token 비면 캘린더 기능 통째 off(회귀 0) |
| scope | `https://www.googleapis.com/auth/calendar.events` |

## 3. 컴포넌트

### 3.1 `internal/gcal` (신규 패키지, 공식 SDK)

```go
// types.go
type Event struct {
    ID       string
    Summary  string
    Start    time.Time
    End      time.Time
    AllDay   bool
    Location string
}

// auth.go
// TokenSource 는 refresh token 으로 access token 을 자동 갱신하는 소스를 만든다.
func TokenSource(ctx context.Context, clientID, clientSecret, refreshToken string) oauth2.TokenSource

// client.go
type Client struct {
    svc        *calendar.Service
    calendarID string
}
func New(ctx context.Context, clientID, clientSecret, refreshToken, calendarID string) (*Client, error)
func (c *Client) ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]Event, error)
func (c *Client) AddEvent(ctx context.Context, ev Event) (Event, error)
func (c *Client) DeleteEvent(ctx context.Context, id string) error
func (c *Client) SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error)
```

- `New`는 `calendar.NewService(ctx, option.WithTokenSource(TokenSource(...)))`로 서비스 생성. `calendarID` 빈 값이면 `"primary"`.
- `ListEvents`/`SearchEvents`: `Events.List(calendarID).TimeMin(RFC3339).TimeMax(RFC3339).SingleEvents(true).OrderBy("startTime")`; Search는 추가로 `.Q(query)`.
- 응답 `*calendar.Event`를 `Event`로 변환(타임드 이벤트는 `Start.DateTime`, 종일은 `Start.Date` → AllDay=true).
- `AddEvent`: 타임드면 `EventDateTime{DateTime, TimeZone}`, 종일이면 `EventDateTime{Date}`.

### 3.2 `cmd/calauth` (1회용 OAuth 부트스트랩)

loopback redirect 흐름(OOB 복붙은 Google이 폐기):
1. `.env`의 `GOOGLE_OAUTH_CLIENT_ID`/`SECRET` 로드.
2. `oauth2.Config{ClientID, ClientSecret, Scopes:[calendar.events], RedirectURL:"http://localhost:8910", Endpoint: google.Endpoint}`.
3. `localhost:8910` 로컬 서버 기동 + `AuthCodeURL(state, AccessTypeOffline, ApprovalForce)` 출력.
4. 브라우저 로그인 → `/?code=...` 리다이렉트 캡처 → `Exchange` → **refresh token 출력**.
5. 사용자가 `.env`에 `GOOGLE_CALENDAR_REFRESH_TOKEN` 붙여넣기.

> Desktop app 타입 OAuth 클라이언트는 loopback 리다이렉트를 허용한다. `AccessTypeOffline`+`ApprovalForce`(prompt=consent)로 refresh token을 반드시 받는다.

### 3.3 `internal/agent/calendar_tools.go`

```go
type CalendarPort interface {
    ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]gcal.Event, error)
    AddEvent(ctx context.Context, ev gcal.Event) (gcal.Event, error)
    DeleteEvent(ctx context.Context, id string) error
    SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]gcal.Event, error)
}
func CalendarTools(port CalendarPort) []Tool // list_events, add_event, search_events (Run); delete_event (Propose)
```

| 도구 | 종류 | 인자 | 동작 |
|---|---|---|---|
| `list_events` | Run | `period`(today/tomorrow/week/month, 기본 week) | 서버가 `time.Now()`로 범위 계산 → ListEvents → 포맷 |
| `add_event` | Run | `summary`(필수), `start`(필수, RFC3339 또는 YYYY-MM-DD), `end`(선택), `location`(선택) | 즉시 AddEvent. start가 날짜만이면 종일, end 없으면 타임드 +1h |
| `search_events` | Run | `query`(필수) | −1년~+1년 창에서 SearchEvents |
| `delete_event` | Propose(Write) | `query`(필수) | 검색으로 1개 resolve → `ChangeProposal{Op:"delete_event", Fields:{event_id, summary}}` |

- delete resolve: 다가오는~최근(예: −30일~+1년) 이벤트에서 query 부분일치. 0개/다수면 되묻기(에러).
- 응답 voice: 존댓말 + 이모지 + `•` 불릿.
- 종일/타임드 시간 표기: 타임드 `M월 D일 HH:MM`, 종일 `M월 D일 (종일)` (Asia/Seoul).

### 3.4 `internal/agent/calendar_applier.go`

```go
type calendarApplier struct{ port CalendarPort }
func NewCalendarApplier(port CalendarPort) domain.ProposalApplier
// Apply: Op "delete_event" 만 처리 → port.DeleteEvent(Fields["event_id"]) → "🗑️ '<summary>' 일정을 삭제했습니다."
```

main의 `NewDispatchApplier(map{"delete_todo": todoistApplier, "delete_event": calendarApplier}, homeApplier)`에 등록.

### 3.5 아침 브리핑 통합

`internal/agent/calendar_briefing.go` 또는 기존 `todoist_briefing.go` 수정:
```go
func NewMorningBriefing(todo TodoistPort, cal CalendarPort, sender domain.MessageSender, channel string) func(ctx context.Context)
```
- `cal != nil`이면 오늘(00:00~24:00) 이벤트 조회 → `📅 *오늘 일정*` 섹션.
- 할일 조회 → `✅ *오늘 할 일과 밀린 일*` 섹션.
- 둘 다 비면 기존 "좋은 하루" 메시지. `cal == nil`이면 기존 동작 그대로(할일만).
- 캘린더 조회 실패는 best-effort(로그 후 할일 섹션만 전송).

### 3.6 설정 (`pkg/config/config.go`)

추가:
```go
GoogleOAuthClientID     string
GoogleOAuthClientSecret string
GoogleCalendarRefreshToken string
GoogleCalendarID        string // 기본 "primary"
```
- `GoogleCalendarID = getenv("GOOGLE_CALENDAR_ID", "primary")`, 나머지는 `os.Getenv`.
- validate 변경 없음(전부 선택). 캘린더 활성 조건 = refresh token 비어있지 않음.

### 3.7 배선 (`cmd/server/main.go`)

```go
var calPort agent.CalendarPort
if cfg.GoogleCalendarRefreshToken != "" {
    cal, err := gcal.New(ctx, cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleCalendarRefreshToken, cfg.GoogleCalendarID)
    if err != nil {
        logger.Error("캘린더 초기화 실패 — 캘린더 기능 비활성", "error", err)
    } else {
        calPort = cal
        tools = append(tools, agent.CalendarTools(cal)...)
    }
}
// dispatch applier 맵에 "delete_event": agent.NewCalendarApplier(calPort) 추가(calPort != nil 일 때)
// 아침 브리핑에 calPort 전달
```

### 3.8 시스템 프롬프트

`agent.New`에 넘기는 시스템 프롬프트(또는 기본 프롬프트)에 추가:
- 현재 날짜/시각(Asia/Seoul) — add_event 상대표현 변환용. (런타임에 주입: 프롬프트 빌드 시 `time.Now()` 문자열 삽입.)
- "급한 일/급한 거를 물으면 캘린더(오늘~내일)와 할일(밀린·오늘·내일)을 모두 확인해 합쳐 답하라."

## 4. 의존성

`go get google.golang.org/api/calendar/v3 golang.org/x/oauth2`로 직접 의존성 승격. 무거운 transitive(grpc/protobuf/x-net 등)는 이미 genai가 끌어와 추가 비용 최소.

## 5. 에러 / 안전

- refresh token 무효 → 첫 API 호출 시 도구가 사용자에게 에러 메시지(서버 안 죽음). 구성 실패는 로그 후 캘린더 기능만 비활성.
- 삭제는 항상 버튼 승인. add/search/list는 즉시.
- 토큰 자동 갱신은 oauth2 TokenSource가 처리(만료 시 refresh token으로 재발급).
- 브리핑의 캘린더 조회 실패는 best-effort(할일만이라도 전송).

## 6. 테스트 전략

- `gcal.Client`: `option.WithHTTPClient(httptest 서버)` 주입 → canned JSON. ListEvents/AddEvent/DeleteEvent/SearchEvents의 요청 파라미터(TimeMin/TimeMax/Q/SingleEvents) + 응답 파싱(타임드/종일 변환) 검증.
- `auth`: TokenSource 빌더가 올바른 config(scope/endpoint) 구성하는지 단위 검증(토큰 교환은 네트워크라 제외).
- `calendar_tools`: fake CalendarPort — list 포맷(타임드/종일), add 즉시(start 파싱: 날짜만→종일, 일시→타임드+1h), search, delete Propose(Op/Fields).
- `calendarApplier`: fake port, delete + reply 문자열.
- 브리핑: fake todo+cal port — 캘린더 유/무, 빈 날, 캘린더 조회 실패 폴백.
- 정적 시간 주입, `t.Parallel()`.

## 7. 범위 밖 (YAGNI)

- 저녁 브리핑 캘린더 통합(메모리는 아침만)
- 일정 수정(update_event) — 추가/삭제로 대체 가능, 후속
- 다중 캘린더, 참석자 초대, 알림 설정
- 반복 일정 생성(단일 이벤트만; 조회는 SingleEvents로 전개됨)
- "급한 일" 전용 합성 도구(에이전트가 조합)
