# Google Calendar 연동 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Google Calendar를 jarvis에 연동해 슬랙 자연어로 일정 조회/추가/삭제/검색하고, 아침 브리핑에 오늘 일정을 포함한다.

**Architecture:** 새 `internal/gcal` 패키지가 공식 SDK(`calendar/v3` + `oauth2`)로 캘린더를 감싼다. `internal/agent`에 CalendarPort + 4개 도구(list/add/search=즉시, delete=버튼승인) + delete applier를 추가하고, 아침 브리핑에 오늘 일정 섹션을 얹는다. 상대 날짜는 에이전트가 시스템 프롬프트에 주입된 현재 시각으로 변환한다. `cmd/calauth`가 1회용 OAuth refresh token을 발급한다.

**Tech Stack:** Go 1.25, `google.golang.org/api/calendar/v3`, `golang.org/x/oauth2`(+`/google`), 기존 `genai`.

## Global Constraints

- Go module `github.com/Jongseong0111/jarvis`, go 1.25.5. Clean Architecture, constructor 주입, value receiver 선호.
- 한국어 주석/커밋. 로깅 stdlib `slog`(`pkg/log`). 에러 `fmt.Errorf("...: %w", err)`.
- 테스트 table-driven + `t.Parallel()` + 정적 시간 주입(`now func() time.Time`).
- 캘린더 활성 조건 = `GOOGLE_CALENDAR_REFRESH_TOKEN` 비어있지 않음. 빈값이면 기능 통째 off(회귀 0). validate() 변경 없음.
- 삭제는 항상 버튼 승인. add/search/list는 즉시 실행. 응답 voice = 존댓말 + 이모지 + `•` 불릿.
- scope = `calendar.CalendarEventsScope` (`https://www.googleapis.com/auth/calendar.events`).
- 대상 캘린더 = `GOOGLE_CALENDAR_ID`(기본 `"primary"`). 타임존 = Asia/Seoul.
- SDK 확인된 사실: `calendar.NewService(ctx, option.WithTokenSource(ts))`; 테스트는 `option.WithHTTPClient(srv.Client())`+`option.WithEndpoint(srv.URL)`+`option.WithoutAuthentication()`. `svc.Events.List(calID).TimeMin(rfc3339).TimeMax(rfc3339).SingleEvents(true).OrderBy("startTime").Q(q).Context(ctx).Do()` → `*calendar.Events{Items []*calendar.Event}`. `svc.Events.Insert(calID, *calendar.Event).Context(ctx).Do()`. `svc.Events.Delete(calID, id).Context(ctx).Do()`. `calendar.EventDateTime{Date, DateTime, TimeZone}`(json date/dateTime/timeZone). `oauth2.Config{ClientID, ClientSecret, Endpoint: google.Endpoint, Scopes, RedirectURL}`; `.TokenSource(ctx, &oauth2.Token{RefreshToken: rt})`; `.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)`; `.Exchange(ctx, code)`.

---

### Task 1: 의존성 추가 + gcal Event 타입 + auth

**Files:**
- Modify: `go.mod`, `go.sum` (go get)
- Create: `internal/gcal/types.go`
- Create: `internal/gcal/auth.go`
- Test: `internal/gcal/auth_test.go`

**Interfaces:**
- Produces:
  - `type Event struct { ID, Summary string; Start, End time.Time; AllDay bool; Location string }`
  - `func oauthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config` (패키지 내부; auth + cmd/calauth 공유 위해 exported alias 아래 참조)
  - `func TokenSource(ctx context.Context, clientID, clientSecret, refreshToken string) oauth2.TokenSource`
  - `func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config` (cmd/calauth 가 쓰도록 export)

- [ ] **Step 1: 의존성 추가**

Run:
```bash
cd ~/personal-agent/jarvis
go get google.golang.org/api/calendar/v3 golang.org/x/oauth2
go mod tidy
```
Expected: go.mod 에 `google.golang.org/api`, `golang.org/x/oauth2` 가 direct require 로 추가됨. (transitive grpc/protobuf/genproto 가 함께 올라갈 수 있음 — Step 5 전체 테스트로 genai 호환 확인.)

- [ ] **Step 2: 실패 테스트 작성**

`internal/gcal/auth_test.go`:
```go
package gcal

import (
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestOAuthConfig(t *testing.T) {
	t.Parallel()
	cfg := OAuthConfig("cid", "secret", "http://localhost:8910")
	if cfg.ClientID != "cid" || cfg.ClientSecret != "secret" {
		t.Fatalf("client id/secret 미설정: %+v", cfg)
	}
	if cfg.RedirectURL != "http://localhost:8910" {
		t.Fatalf("redirect url = %q", cfg.RedirectURL)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != calendar.CalendarEventsScope {
		t.Fatalf("scope = %v, want [%s]", cfg.Scopes, calendar.CalendarEventsScope)
	}
	if !strings.Contains(cfg.Endpoint.AuthURL, "google") {
		t.Fatalf("endpoint AuthURL = %q (google 아님)", cfg.Endpoint.AuthURL)
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/gcal/ -run TestOAuthConfig -v`
Expected: FAIL (`undefined: OAuthConfig`).

- [ ] **Step 4: 구현**

`internal/gcal/types.go`:
```go
// Package gcal 은 Google Calendar(공식 SDK)를 감싸는 어댑터다.
package gcal

import "time"

// Event 는 캘린더 일정의 도메인 표현이다.
type Event struct {
	ID       string
	Summary  string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Location string
}
```

`internal/gcal/auth.go`:
```go
package gcal

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

// OAuthConfig 는 캘린더용 OAuth2 설정을 만든다(cmd/calauth 와 공유).
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{calendar.CalendarEventsScope},
		RedirectURL:  redirectURL,
	}
}

// TokenSource 는 refresh token 으로 access token 을 자동 갱신하는 소스를 만든다.
func TokenSource(ctx context.Context, clientID, clientSecret, refreshToken string) oauth2.TokenSource {
	cfg := OAuthConfig(clientID, clientSecret, "")
	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
}
```

- [ ] **Step 5: 테스트 통과 + 전체 회귀 확인(중요)**

Run:
```bash
cd ~/personal-agent/jarvis && go test ./internal/gcal/ -run TestOAuthConfig -v && go build ./... && go test ./... 2>&1 | tail -20
```
Expected: PASS + 전 패키지 green. **특히 `internal/gemini`/`internal/agent` 가 dep 업그레이드 후에도 통과해야 함**(genai 호환). 깨지면 BLOCKED 보고.

- [ ] **Step 6: 커밋**

```bash
cd ~/personal-agent/jarvis
git add go.mod go.sum internal/gcal/types.go internal/gcal/auth.go internal/gcal/auth_test.go
git commit -m "feat(gcal): calendar/oauth2 의존성 + Event 타입 + OAuth TokenSource"
```

---

### Task 2: gcal Client (List/Add/Delete/Search)

**Files:**
- Create: `internal/gcal/client.go`
- Test: `internal/gcal/client_test.go`

**Interfaces:**
- Consumes: `Event` (Task 1).
- Produces:
  - `type Client struct { svc *calendar.Service; calendarID string }`
  - `func New(ctx context.Context, clientID, clientSecret, refreshToken, calendarID string) (*Client, error)`
  - `func newWithService(svc *calendar.Service, calendarID string) *Client` (테스트 주입용, 내부)
  - `func (c *Client) ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]Event, error)`
  - `func (c *Client) SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error)`
  - `func (c *Client) AddEvent(ctx context.Context, ev Event) (Event, error)`
  - `func (c *Client) DeleteEvent(ctx context.Context, id string) error`

- [ ] **Step 1: 실패 테스트 작성**

`internal/gcal/client_test.go`:
```go
package gcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// testClient 는 httptest 서버에 붙은 Client 를 만든다. handler 가 응답을 결정한다.
func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	svc, err := calendar.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return newWithService(svc, "primary"), srv
}

func TestListEvents_ParsesTimedAndAllDay(t *testing.T) {
	t.Parallel()
	var gotPath, gotQuery string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(calendar.Events{Items: []*calendar.Event{
			{Id: "e1", Summary: "미팅", Location: "회의실",
				Start: &calendar.EventDateTime{DateTime: "2026-06-23T15:00:00+09:00"},
				End:   &calendar.EventDateTime{DateTime: "2026-06-23T16:00:00+09:00"}},
			{Id: "e2", Summary: "아기 검진",
				Start: &calendar.EventDateTime{Date: "2026-06-24"},
				End:   &calendar.EventDateTime{Date: "2026-06-25"}},
		}})
	})
	defer srv.Close()

	from := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	evs, err := c.ListEvents(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if !strings.Contains(gotPath, "/calendars/primary/events") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "singleEvents=true") || !strings.Contains(gotQuery, "orderBy=startTime") {
		t.Fatalf("query = %q (singleEvents/orderBy 누락)", gotQuery)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}
	if evs[0].AllDay || evs[0].Summary != "미팅" || evs[0].Location != "회의실" {
		t.Fatalf("timed event 파싱 오류: %+v", evs[0])
	}
	if !evs[1].AllDay || evs[1].Summary != "아기 검진" {
		t.Fatalf("all-day event 파싱 오류: %+v", evs[1])
	}
}

func TestSearchEvents_SendsQuery(t *testing.T) {
	t.Parallel()
	var gotQuery string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(calendar.Events{Items: nil})
	})
	defer srv.Close()
	_, err := c.SearchEvents(context.Background(), "검진", time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SearchEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "q=") || !strings.Contains(gotQuery, "%EA%B2%80%EC%A7%84") {
		t.Fatalf("query 에 q=검진 누락: %q", gotQuery)
	}
}

func TestAddEvent_Timed(t *testing.T) {
	t.Parallel()
	var body calendar.Event
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.Id = "new1"
		_ = json.NewEncoder(w).Encode(body)
	})
	defer srv.Close()
	start := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)
	out, err := c.AddEvent(context.Background(), Event{Summary: "회의", Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if out.ID != "new1" {
		t.Fatalf("반환 ID = %q", out.ID)
	}
	if body.Summary != "회의" || body.Start.DateTime == "" {
		t.Fatalf("요청 body 오류(타임드 DateTime 누락): %+v", body.Start)
	}
}

func TestAddEvent_AllDay(t *testing.T) {
	t.Parallel()
	var body calendar.Event
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(body)
	})
	defer srv.Close()
	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	_, err := c.AddEvent(context.Background(), Event{Summary: "검진", Start: day, End: day.AddDate(0, 0, 1), AllDay: true})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if body.Start.Date != "2026-07-03" || body.Start.DateTime != "" {
		t.Fatalf("종일 이벤트는 Date 만 채워야 함: %+v", body.Start)
	}
}

func TestDeleteEvent(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()
	if err := c.DeleteEvent(context.Background(), "e9"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if gotMethod != http.MethodDelete || !strings.Contains(gotPath, "/events/e9") {
		t.Fatalf("delete 요청 오류: %s %s", gotMethod, gotPath)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/gcal/ -run 'TestListEvents|TestSearch|TestAdd|TestDelete' -v`
Expected: FAIL (`undefined: newWithService` 등).

- [ ] **Step 3: 구현**

`internal/gcal/client.go`:
```go
package gcal

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const seoulTZName = "Asia/Seoul"

// Client 는 Google Calendar 호출을 감싼다.
type Client struct {
	svc        *calendar.Service
	calendarID string
}

// New 는 refresh token 기반 Client 를 만든다. calendarID 빈 값이면 "primary".
func New(ctx context.Context, clientID, clientSecret, refreshToken, calendarID string) (*Client, error) {
	svc, err := calendar.NewService(ctx, option.WithTokenSource(
		TokenSource(ctx, clientID, clientSecret, refreshToken)))
	if err != nil {
		return nil, fmt.Errorf("calendar 서비스 생성 실패: %w", err)
	}
	return newWithService(svc, calendarID), nil
}

func newWithService(svc *calendar.Service, calendarID string) *Client {
	if calendarID == "" {
		calendarID = "primary"
	}
	return &Client{svc: svc, calendarID: calendarID}
}

// ListEvents 는 [timeMin, timeMax) 의 일정을 시작시각 순으로 조회한다.
func (c *Client) ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]Event, error) {
	return c.list(ctx, "", timeMin, timeMax)
}

// SearchEvents 는 query 로 일정을 검색한다.
func (c *Client) SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error) {
	return c.list(ctx, query, timeMin, timeMax)
}

func (c *Client) list(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error) {
	call := c.svc.Events.List(c.calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime")
	if query != "" {
		call = call.Q(query)
	}
	res, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("일정 조회 실패: %w", err)
	}
	out := make([]Event, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, toEvent(it))
	}
	return out, nil
}

// AddEvent 는 일정을 등록하고 등록된 일정을 반환한다.
func (c *Client) AddEvent(ctx context.Context, ev Event) (Event, error) {
	created, err := c.svc.Events.Insert(c.calendarID, toCalendarEvent(ev)).Context(ctx).Do()
	if err != nil {
		return Event{}, fmt.Errorf("일정 등록 실패: %w", err)
	}
	return toEvent(created), nil
}

// DeleteEvent 는 일정을 삭제한다.
func (c *Client) DeleteEvent(ctx context.Context, id string) error {
	if err := c.svc.Events.Delete(c.calendarID, id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("일정 삭제 실패: %w", err)
	}
	return nil
}

// toEvent 는 SDK 이벤트를 도메인 Event 로 변환한다(타임드/종일 구분).
func toEvent(e *calendar.Event) Event {
	ev := Event{ID: e.Id, Summary: e.Summary, Location: e.Location}
	if e.Start != nil {
		if e.Start.Date != "" { // 종일
			ev.AllDay = true
			ev.Start, _ = time.Parse("2006-01-02", e.Start.Date)
			if e.End != nil && e.End.Date != "" {
				ev.End, _ = time.Parse("2006-01-02", e.End.Date)
			}
		} else {
			ev.Start, _ = time.Parse(time.RFC3339, e.Start.DateTime)
			if e.End != nil && e.End.DateTime != "" {
				ev.End, _ = time.Parse(time.RFC3339, e.End.DateTime)
			}
		}
	}
	return ev
}

// toCalendarEvent 는 도메인 Event 를 SDK 등록용으로 변환한다.
func toCalendarEvent(ev Event) *calendar.Event {
	out := &calendar.Event{Summary: ev.Summary, Location: ev.Location}
	if ev.AllDay {
		out.Start = &calendar.EventDateTime{Date: ev.Start.Format("2006-01-02")}
		out.End = &calendar.EventDateTime{Date: ev.End.Format("2006-01-02")}
	} else {
		out.Start = &calendar.EventDateTime{DateTime: ev.Start.Format(time.RFC3339), TimeZone: seoulTZName}
		out.End = &calendar.EventDateTime{DateTime: ev.End.Format(time.RFC3339), TimeZone: seoulTZName}
	}
	return out
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/gcal/ -v`
Expected: PASS (전 gcal 테스트).

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/gcal/client.go internal/gcal/client_test.go
git commit -m "feat(gcal): Client — List/Search/Add/Delete + 타임드·종일 변환"
```

---

### Task 3: cmd/calauth (OAuth 부트스트랩)

**Files:**
- Create: `cmd/calauth/main.go`
- Test: `cmd/calauth/main_test.go`

**Interfaces:**
- Consumes: `gcal.OAuthConfig` (Task 1).
- Produces: 실행형 main. 순수 헬퍼 `func randomState() (string, error)` 만 단위 테스트.

- [ ] **Step 1: 실패 테스트 작성**

`cmd/calauth/main_test.go`:
```go
package main

import "testing"

func TestRandomState(t *testing.T) {
	t.Parallel()
	a, err := randomState()
	if err != nil {
		t.Fatalf("randomState: %v", err)
	}
	b, _ := randomState()
	if a == "" || a == b {
		t.Fatalf("state 가 비었거나 매번 같음: %q vs %q", a, b)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./cmd/calauth/ -run TestRandomState -v`
Expected: FAIL (`undefined: randomState`).

- [ ] **Step 3: 구현**

`cmd/calauth/main.go`:
```go
// Command calauth 는 Google Calendar refresh token 을 1회 발급한다.
// 사용법: config/.env 에 GOOGLE_OAUTH_CLIENT_ID/SECRET 기입 후 `go run ./cmd/calauth`.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/Jongseong0111/jarvis/internal/gcal"
)

const redirectURL = "http://localhost:8910"

func main() {
	_ = godotenv.Load("config/.env")
	clientID := os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	secret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	if clientID == "" || secret == "" {
		log.Fatal("GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET 를 config/.env 에 먼저 넣어주세요.")
	}

	cfg := gcal.OAuthConfig(clientID, secret, redirectURL)
	state, err := randomState()
	if err != nil {
		log.Fatalf("state 생성 실패: %v", err)
	}

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8910"}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state 불일치", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "인증 완료. 터미널로 돌아가세요.")
		codeCh <- code
	})
	go func() { _ = srv.ListenAndServe() }()

	authURL := cfg.AuthCodeURL(state, /* offline+consent: refresh token 보장 */)
	fmt.Println("아래 URL 을 브라우저에서 열어 로그인/동의하세요:")
	fmt.Println(authURL)

	code := <-codeCh
	_ = srv.Shutdown(context.Background())

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("토큰 교환 실패: %v", err)
	}
	if tok.RefreshToken == "" {
		log.Fatal("refresh token 이 비었습니다. 동의 화면에서 offline access 를 허용했는지 확인하세요.")
	}
	fmt.Println("\n✅ 아래를 config/.env 에 추가하세요:")
	fmt.Printf("GOOGLE_CALENDAR_REFRESH_TOKEN=%s\n", tok.RefreshToken)
}

// randomState 는 CSRF 방지용 무작위 state 를 만든다.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

> AuthCodeURL 호출에 `oauth2.AccessTypeOffline, oauth2.ApprovalForce` 를 넣어야 refresh token 이 보장된다. `golang.org/x/oauth2` 를 import 하고 `cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)` 로 작성하라(위 주석 자리). import 블록에 `"golang.org/x/oauth2"` 추가.

- [ ] **Step 4: 테스트 통과 + 빌드/vet**

Run: `cd ~/personal-agent/jarvis && go test ./cmd/calauth/ -v && go build ./... && go vet ./...`
Expected: PASS + 빌드/vet 클린.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add cmd/calauth/
git commit -m "feat(calauth): 1회용 OAuth refresh token 발급 커맨드(loopback)"
```

---

### Task 4: 에이전트 현재시각 주입 + CalendarSystemHint

**Files:**
- Modify: `internal/agent/agent.go`
- Test: `internal/agent/agent_datetime_test.go`

**Interfaces:**
- Produces:
  - `Agent.now func() time.Time` 필드(기본 `time.Now`, 테스트 주입).
  - 내부 `func (a Agent) datedSystem() string` — `a.system` + 현재 시각(Asia/Seoul) 줄.
  - `const CalendarSystemHint string` — 캘린더 활성 시 main 이 시스템 프롬프트에 덧붙일 지시문.
- Route 가 `a.gen.GenerateWithTools` 에 `a.system` 대신 `a.datedSystem()` 결과를 넘김.

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/agent_datetime_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

// sysCapturingGen 은 넘어온 system 문자열을 기록하는 fake generator 다.
type sysCapturingGen struct{ gotSystem string }

func (g *sysCapturingGen) GenerateWithTools(ctx context.Context, contents []*genai.Content, tools []*genai.Tool, system string) (*genai.GenerateContentResponse, error) {
	g.gotSystem = system
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Parts: []*genai.Part{{Text: "네"}}},
	}}}, nil
}

func TestRoute_InjectsCurrentDate(t *testing.T) {
	t.Parallel()
	gen := &sysCapturingGen{}
	ag := New(gen, nil, nil, "")
	ag.now = func() time.Time { return time.Date(2026, 6, 22, 14, 30, 0, 0, time.UTC) }
	_, err := ag.Route(context.Background(), domain.IncomingMessage{ChannelID: "c1", Text: "안녕"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !strings.Contains(gen.gotSystem, "2026-06-22") {
		t.Fatalf("system 에 현재 날짜 미주입:\n%s", gen.gotSystem)
	}
	if !strings.Contains(gen.gotSystem, DefaultSystemPrompt) {
		t.Fatal("system 에 기본 프롬프트가 보존되지 않음")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestRoute_InjectsCurrentDate -v`
Expected: FAIL (`ag.now undefined`).

- [ ] **Step 3: 구현**

`internal/agent/agent.go`:

import 에 `"time"` 추가.

`Agent` 구조체에 필드 추가:
```go
type Agent struct {
	gen    generator
	vision VisionExtractor
	tools  map[string]Tool
	decls  []*genai.Tool
	system string
	now    func() time.Time
	mem    *memory
}
```

`New` 에서 now 기본값 설정:
```go
	return Agent{gen: gen, vision: vision, tools: toolMap(tools), decls: toolDecls(tools), system: system, now: time.Now, mem: newMemory()}
```

파일에 헬퍼 추가(예: New 아래):
```go
// CalendarSystemHint 는 캘린더 기능이 켜졌을 때 시스템 프롬프트에 덧붙이는 지시문이다.
const CalendarSystemHint = `
- 일정/캘린더 조회·추가·삭제·검색은 캘린더 도구(list_events/add_event/search_events/delete_event)를 쓴다. add_event 의 start/end 는 위 '현재 시각'을 기준으로 상대 표현을 RFC3339(예: 2026-06-29T15:00:00+09:00)나 종일이면 YYYY-MM-DD 로 변환해 넣는다.
- "급한 일/급한 거"를 물으면 캘린더(오늘~내일)와 할일(밀린·오늘·내일)을 모두 확인해 합쳐서 답한다.
- 일정 삭제는 delete_event 로 변경안을 만들어 승인 버튼을 거친다.`

// datedSystem 은 기본 시스템 프롬프트에 현재 시각(Asia/Seoul)을 덧붙인다.
// 서버가 장시간 떠 있어도 메시지마다 최신 날짜가 들어가도록 호출 시점에 계산한다.
func (a Agent) datedSystem() string {
	t := a.now()
	if loc, err := time.LoadLocation("Asia/Seoul"); err == nil {
		t = t.In(loc)
	}
	return a.system + "\n\n[현재 시각: " + t.Format("2006-01-02 (Mon) 15:04") + " Asia/Seoul. 상대적 날짜·시간 표현은 이 시각 기준으로 해석한다.]"
}
```

`Route` 의 루프에서 시스템 문자열 교체. 루프 직전에 한 번 계산:
```go
	contents := append(a.mem.get(in.ChannelID), genai.Text(in.Text)...)
	lastResult := ""
	system := a.datedSystem()

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.gen.GenerateWithTools(ctx, contents, a.decls, system)
```
(즉 `a.system` → `system`.)

- [ ] **Step 4: 테스트 통과 + 회귀 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -v 2>&1 | tail -20`
Expected: PASS. **기존 agent 테스트 중 system 문자열을 정확 비교하는 게 있으면 깨질 수 있다** — 있으면 그 테스트가 `datedSystem()` 또는 `strings.Contains` 로 기대하도록 갱신(정적 now 주입). 단순 무시하는 fake면 영향 없음.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/agent/agent.go internal/agent/agent_datetime_test.go
git commit -m "feat(agent): 메시지마다 현재 시각 주입(상대날짜 변환) + CalendarSystemHint"
```

---

### Task 5: calendar_tools (CalendarPort + 4 도구)

**Files:**
- Create: `internal/agent/calendar_tools.go`
- Test: `internal/agent/calendar_tools_test.go`

**Interfaces:**
- Consumes: `gcal.Event` (Task 1); `Tool`, `strArg`, `objSchema`, `strSchema` (`internal/agent/tools.go`).
- Produces:
  - `type CalendarPort interface { ListEvents(ctx, timeMin, timeMax time.Time) ([]gcal.Event, error); AddEvent(ctx, gcal.Event) (gcal.Event, error); DeleteEvent(ctx, id string) error; SearchEvents(ctx, query string, timeMin, timeMax time.Time) ([]gcal.Event, error) }`
  - `func CalendarTools(port CalendarPort) []Tool`
  - 내부: `func eventRange(now time.Time, period string) (timeMin, timeMax time.Time)`; `func parseEventTimes(startStr, endStr string) (start, end time.Time, allDay bool, err error)`; `func formatEvents(evs []gcal.Event) string`.
- `calendar_tools.go` 가 도구 내부에서 `time.Now()` 를 쓰되, 패키지 변수 `var calNow = time.Now` 로 두어 테스트가 교체할 수 있게 한다.

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/calendar_tools_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/internal/gcal"
)

type fakeCalPort struct {
	events    []gcal.Event
	added     gcal.Event
	deletedID string
	lastMin   time.Time
	lastMax   time.Time
	lastQuery string
}

func (f *fakeCalPort) ListEvents(ctx context.Context, mn, mx time.Time) ([]gcal.Event, error) {
	f.lastMin, f.lastMax = mn, mx
	return f.events, nil
}
func (f *fakeCalPort) SearchEvents(ctx context.Context, q string, mn, mx time.Time) ([]gcal.Event, error) {
	f.lastQuery = q
	return f.events, nil
}
func (f *fakeCalPort) AddEvent(ctx context.Context, ev gcal.Event) (gcal.Event, error) {
	f.added = ev
	ev.ID = "new1"
	return ev, nil
}
func (f *fakeCalPort) DeleteEvent(ctx context.Context, id string) error {
	f.deletedID = id
	return nil
}

func toolByName(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Decl.Name == name {
			return t
		}
	}
	return Tool{}
}

func TestListEventsTool_Format(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{events: []gcal.Event{
		{Summary: "미팅", Start: time.Date(2026, 6, 23, 15, 0, 0, 0, time.UTC)},
		{Summary: "검진", AllDay: true, Start: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)},
	}}
	tool := toolByName(CalendarTools(port), "list_events")
	out, err := tool.Run(context.Background(), map[string]any{"period": "week"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"미팅", "검진", "•"} {
		if !strings.Contains(out, want) {
			t.Fatalf("출력에 %q 없음:\n%s", want, out)
		}
	}
}

func TestAddEventTool_TimedAndAllDay(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{}
	tool := toolByName(CalendarTools(port), "add_event")

	// 타임드: end 없으면 +1h
	if _, err := tool.Run(context.Background(), map[string]any{"summary": "회의", "start": "2026-06-29T15:00:00+09:00"}); err != nil {
		t.Fatalf("Run timed: %v", err)
	}
	if port.added.AllDay || port.added.Summary != "회의" {
		t.Fatalf("타임드 add 오류: %+v", port.added)
	}
	if got := port.added.End.Sub(port.added.Start); got != time.Hour {
		t.Fatalf("기본 종료가 +1h 아님: %v", got)
	}

	// 종일: 날짜만
	if _, err := tool.Run(context.Background(), map[string]any{"summary": "검진", "start": "2026-07-03"}); err != nil {
		t.Fatalf("Run all-day: %v", err)
	}
	if !port.added.AllDay {
		t.Fatalf("종일 플래그 미설정: %+v", port.added)
	}
}

func TestDeleteEventTool_Proposes(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{events: []gcal.Event{{ID: "e9", Summary: "치과 예약", Start: time.Now().Add(48 * time.Hour)}}}
	tool := toolByName(CalendarTools(port), "delete_event")
	if !tool.Write {
		t.Fatal("delete_event 는 Write=true 여야 함")
	}
	p, err := tool.Propose(context.Background(), map[string]any{"query": "치과"})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if p.Op != "delete_event" || p.Fields["event_id"] != "e9" {
		t.Fatalf("변경안 오류: %+v", p)
	}
}

func TestEventRange(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC)
	min, max := eventRange(now, "today")
	if min.Day() != 22 || max.Sub(min) != 24*time.Hour {
		t.Fatalf("today 범위 오류: %v ~ %v", min, max)
	}
	_, wmax := eventRange(now, "week")
	if wmax.Sub(now) < 6*24*time.Hour {
		t.Fatalf("week 범위가 너무 짧음: %v", wmax.Sub(now))
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run 'TestListEventsTool|TestAddEventTool|TestDeleteEventTool|TestEventRange' -v`
Expected: FAIL (`undefined: CalendarTools`).

- [ ] **Step 3: 구현**

`internal/agent/calendar_tools.go`:
```go
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/gcal"
)

// calNow 는 캘린더 도구가 쓰는 현재 시각이다(테스트에서 교체).
var calNow = time.Now

// CalendarPort 는 캘린더 조작 능력이다(gcal.Client 가 구현).
type CalendarPort interface {
	ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]gcal.Event, error)
	SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]gcal.Event, error)
	AddEvent(ctx context.Context, ev gcal.Event) (gcal.Event, error)
	DeleteEvent(ctx context.Context, id string) error
}

type calendarTools struct{ port CalendarPort }

// CalendarTools 는 일정 도구 목록을 만든다.
// list/add/search 는 즉시 실행(Run), delete 만 변경안(Propose).
func CalendarTools(port CalendarPort) []Tool {
	t := calendarTools{port: port}
	return []Tool{t.listEvents(), t.addEvent(), t.searchEvents(), t.deleteEvent()}
}

func (t calendarTools) listEvents() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_events",
			Description: "캘린더 일정을 조회한다. period: today, tomorrow, week(기본), month.",
			Parameters: objSchema(map[string]*genai.Schema{
				"period": strSchema("today/tomorrow/week/month 중 하나(기본 week)"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			period := strings.TrimSpace(strArg(args, "period"))
			if period == "" {
				period = "week"
			}
			mn, mx := eventRange(calNow(), period)
			evs, err := t.port.ListEvents(ctx, mn, mx)
			if err != nil {
				return "", err
			}
			if len(evs) == 0 {
				return "📅 해당 기간에 일정이 없습니다.", nil
			}
			return "📅 *일정*\n" + formatEvents(evs), nil
		},
	}
}

func (t calendarTools) addEvent() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "add_event",
			Description: "캘린더에 일정을 추가한다. start 는 RFC3339(예: 2026-06-29T15:00:00+09:00) 또는 종일이면 YYYY-MM-DD. end 는 선택.",
			Parameters: objSchema(map[string]*genai.Schema{
				"summary":  strSchema("일정 제목"),
				"start":    strSchema("시작. RFC3339 또는 YYYY-MM-DD(종일)"),
				"end":      strSchema("종료(선택). 형식은 start 와 동일"),
				"location": strSchema("장소(선택)"),
			}, "summary", "start"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			summary := strings.TrimSpace(strArg(args, "summary"))
			if summary == "" {
				return "", fmt.Errorf("일정 제목을 알려주세요.")
			}
			start, end, allDay, err := parseEventTimes(strArg(args, "start"), strArg(args, "end"))
			if err != nil {
				return "", err
			}
			ev, err := t.port.AddEvent(ctx, gcal.Event{
				Summary: summary, Start: start, End: end, AllDay: allDay,
				Location: strings.TrimSpace(strArg(args, "location")),
			})
			if err != nil {
				return "", err
			}
			return "✅ 일정을 추가했습니다.\n" + formatEventLine(ev), nil
		},
	}
}

func (t calendarTools) searchEvents() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "search_events",
			Description: "키워드로 일정을 검색한다(과거~미래). 예: '아기 검진 언제였지?'",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("검색어"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			q := strings.TrimSpace(strArg(args, "query"))
			if q == "" {
				return "", fmt.Errorf("검색어를 알려주세요.")
			}
			now := calNow()
			evs, err := t.port.SearchEvents(ctx, q, now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0))
			if err != nil {
				return "", err
			}
			if len(evs) == 0 {
				return fmt.Sprintf("🔍 '%s' 일정을 찾지 못했습니다.", q), nil
			}
			return "🔍 *검색 결과*\n" + formatEvents(evs), nil
		},
	}
}

func (t calendarTools) deleteEvent() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_event",
			Description: "일정을 삭제한다. query 로 대상을 찾아 승인 버튼을 거친다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("삭제할 일정 키워드(부분일치)"),
			}, "query"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			ev, err := t.resolveEvent(ctx, strArg(args, "query"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			return domain.ChangeProposal{
				Op:      "delete_event",
				Summary: "🗑️ 다음 일정을 삭제할까요?\n" + formatEventLine(ev),
				Fields:  map[string]string{"event_id": ev.ID, "summary": ev.Summary},
			}, nil
		},
	}
}

// resolveEvent 는 query 로 일정 1개를 찾는다(최근 30일~+1년). 0개/다수면 에러(되묻기).
func (t calendarTools) resolveEvent(ctx context.Context, query string) (gcal.Event, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return gcal.Event{}, fmt.Errorf("어떤 일정인지 알려주세요.")
	}
	now := calNow()
	evs, err := t.port.SearchEvents(ctx, query, now.AddDate(0, 0, -30), now.AddDate(1, 0, 0))
	if err != nil {
		return gcal.Event{}, err
	}
	switch len(evs) {
	case 0:
		return gcal.Event{}, fmt.Errorf("'%s'에 해당하는 일정을 찾지 못했습니다.", query)
	case 1:
		return evs[0], nil
	default:
		var names []string
		for _, e := range evs {
			names = append(names, e.Summary)
		}
		return gcal.Event{}, fmt.Errorf("'%s'에 해당하는 일정이 여러 개예요: %s. 더 정확히 알려주세요.", query, strings.Join(names, ", "))
	}
}

// eventRange 는 period 에 대한 [timeMin, timeMax) 를 now 기준으로 만든다.
func eventRange(now time.Time, period string) (timeMin, timeMax time.Time) {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch period {
	case "today":
		return start, start.AddDate(0, 0, 1)
	case "tomorrow":
		return start.AddDate(0, 0, 1), start.AddDate(0, 0, 2)
	case "month":
		return start, start.AddDate(0, 0, 30)
	default: // week
		return start, start.AddDate(0, 0, 7)
	}
}

// parseEventTimes 는 start/end 문자열을 파싱한다. YYYY-MM-DD 면 종일, RFC3339 면 타임드.
// end 가 비면 타임드는 +1h, 종일은 +1일.
func parseEventTimes(startStr, endStr string) (start, end time.Time, allDay bool, err error) {
	startStr = strings.TrimSpace(startStr)
	endStr = strings.TrimSpace(endStr)
	if startStr == "" {
		return start, end, false, fmt.Errorf("시작 시각을 알려주세요.")
	}
	if len(startStr) == 10 { // YYYY-MM-DD
		allDay = true
		start, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			return start, end, false, fmt.Errorf("날짜 형식 오류(YYYY-MM-DD): %q", startStr)
		}
		if endStr != "" {
			end, err = time.Parse("2006-01-02", endStr)
			if err != nil {
				return start, end, false, fmt.Errorf("종료 날짜 형식 오류: %q", endStr)
			}
		} else {
			end = start.AddDate(0, 0, 1)
		}
		return start, end, true, nil
	}
	start, err = time.Parse(time.RFC3339, startStr)
	if err != nil {
		return start, end, false, fmt.Errorf("시작 시각 형식 오류(RFC3339): %q", startStr)
	}
	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return start, end, false, fmt.Errorf("종료 시각 형식 오류: %q", endStr)
		}
	} else {
		end = start.Add(time.Hour)
	}
	return start, end, false, nil
}

// formatEvents 는 일정 목록을 • 불릿으로 만든다.
func formatEvents(evs []gcal.Event) string {
	var lines []string
	for _, e := range evs {
		lines = append(lines, formatEventLine(e))
	}
	return strings.Join(lines, "\n")
}

// formatEventLine 은 일정 1건을 한 줄로 만든다(Asia/Seoul 표기).
func formatEventLine(e gcal.Event) string {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	s := e.Start.In(loc)
	when := s.Format("1월 2일 (Mon) 15:04")
	if e.AllDay {
		when = s.Format("1월 2일") + " (종일)"
	}
	line := "• " + when + " — " + e.Summary
	if e.Location != "" {
		line += " @" + e.Location
	}
	return line
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run 'TestListEventsTool|TestAddEventTool|TestDeleteEventTool|TestEventRange' -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/agent/calendar_tools.go internal/agent/calendar_tools_test.go
git commit -m "feat(agent): 캘린더 도구(list/add/search 즉시, delete 버튼승인) + 시각 파싱/포맷"
```

---

### Task 6: calendar_applier (delete_event 적용)

**Files:**
- Create: `internal/agent/calendar_applier.go`
- Test: `internal/agent/calendar_applier_test.go`

**Interfaces:**
- Consumes: `CalendarPort` (Task 5); `domain.ChangeProposal`, `domain.ProposalApplier`, `domain.Reply`.
- Produces: `func NewCalendarApplier(port CalendarPort) domain.ProposalApplier`. Op `"delete_event"` 만 처리.

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/calendar_applier_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func TestCalendarApplier_Delete(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{}
	ap := NewCalendarApplier(port)
	reply, err := ap.Apply(context.Background(), domain.ChangeProposal{
		Op:     "delete_event",
		Fields: map[string]string{"event_id": "e9", "summary": "치과 예약"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if port.deletedID != "e9" {
		t.Fatalf("삭제된 ID = %q, want e9", port.deletedID)
	}
	if !strings.Contains(reply.Text, "치과 예약") {
		t.Fatalf("응답에 제목 없음: %q", reply.Text)
	}
}

func TestCalendarApplier_WrongOp(t *testing.T) {
	t.Parallel()
	ap := NewCalendarApplier(&fakeCalPort{})
	if _, err := ap.Apply(context.Background(), domain.ChangeProposal{Op: "delete_todo"}); err == nil {
		t.Fatal("지원하지 않는 op 인데 에러가 없음")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestCalendarApplier -v`
Expected: FAIL (`undefined: NewCalendarApplier`).

- [ ] **Step 3: 구현**

`internal/agent/calendar_applier.go`:
```go
package agent

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
)

// calendarApplier 는 delete_event 변경안을 캘린더에 반영한다.
type calendarApplier struct{ port CalendarPort }

// NewCalendarApplier 는 일정 삭제 승인 처리기를 만든다.
func NewCalendarApplier(port CalendarPort) domain.ProposalApplier {
	return calendarApplier{port: port}
}

func (a calendarApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if p.Op != "delete_event" {
		return domain.Reply{}, fmt.Errorf("calendarApplier: 지원하지 않는 op %q", p.Op)
	}
	if err := a.port.DeleteEvent(ctx, p.Fields["event_id"]); err != nil {
		return domain.Reply{}, err
	}
	return domain.Reply{Text: "🗑️ '" + p.Fields["summary"] + "' 일정을 삭제했습니다."}, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestCalendarApplier -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/agent/calendar_applier.go internal/agent/calendar_applier_test.go
git commit -m "feat(agent): calendarApplier — delete_event 승인 적용"
```

---

### Task 7: config 추가

**Files:**
- Modify: `pkg/config/config.go`
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Produces: `Config.GoogleOAuthClientID`, `Config.GoogleOAuthClientSecret`, `Config.GoogleCalendarRefreshToken`, `Config.GoogleCalendarID`(기본 `"primary"`). validate 변경 없음.

- [ ] **Step 1: 실패 테스트 작성**

`pkg/config/config_test.go` 에 추가(기존 `setRequiredEnv` 헬퍼 사용 — 없으면 기존 테스트가 쓰는 필수 env 설정 방식을 그대로 따른다):
```go
func TestCalendarConfigDefault(t *testing.T) {
	setRequiredEnv(t) // 기존 헬퍼: 필수 env 채워 validate 통과
	t.Setenv("GOOGLE_CALENDAR_ID", "")           // godotenv 주입 차단(빈→기본)
	t.Setenv("GOOGLE_CALENDAR_REFRESH_TOKEN", "") // 차단
	cfg, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if cfg.GoogleCalendarID != "primary" {
		t.Fatalf("GoogleCalendarID = %q, want primary", cfg.GoogleCalendarID)
	}
}
```
> 기존 config_test.go 에 `setRequiredEnv` 가 없으면, 다른 테스트가 필수 env(SLACK_*, GEMINI_API_KEY, NOTION_*)를 어떻게 채우는지 보고 동일하게 한다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/ -run TestCalendarConfigDefault -v`
Expected: FAIL (`cfg.GoogleCalendarID undefined`).

- [ ] **Step 3: 구현**

`pkg/config/config.go` `Config` 구조체에 추가:
```go
	GoogleOAuthClientID        string
	GoogleOAuthClientSecret    string
	GoogleCalendarRefreshToken string
	GoogleCalendarID           string // 기본 "primary"
```

`New()` 의 `cfg := Config{...}` 안에 추가:
```go
		GoogleOAuthClientID:        os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret:    os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleCalendarRefreshToken: os.Getenv("GOOGLE_CALENDAR_REFRESH_TOKEN"),
		GoogleCalendarID:           getenv("GOOGLE_CALENDAR_ID", "primary"),
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `cd ~/personal-agent/jarvis && go test ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add pkg/config/
git commit -m "feat(config): Google Calendar 설정(client/secret/refresh_token/calendar_id)"
```

---

### Task 8: 아침 브리핑에 오늘 일정 통합

**Files:**
- Modify: `internal/agent/todoist_briefing.go`
- Modify: `cmd/server/main.go` (NewMorningBriefing 호출부에 nil 추가 — 컴파일 유지)
- Test: `internal/agent/calendar_briefing_test.go`

**Interfaces:**
- Consumes: `CalendarPort` (Task 5), `TodoistPort`, `domain.MessageSender`.
- Produces: `NewMorningBriefing` 시그니처 변경 → `func NewMorningBriefing(port TodoistPort, cal CalendarPort, sender domain.MessageSender, channel string) func(ctx context.Context)`.
  - 내부: `func todayEventLines(ctx, cal CalendarPort) (string, bool)` — calendar nil 이거나 조회 실패면 ("", false)(best-effort).
  - 패키지 변수 `var briefingNow = time.Now`(테스트 주입; 이미 다른 변수명 있으면 calNow 재사용).

- [ ] **Step 1: 실패 테스트 작성**

`internal/agent/calendar_briefing_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
)

// capSender 는 전송된 메시지를 잡는 fake sender 다.
type capSender struct{ last domain.Reply }

func (s *capSender) Send(ctx context.Context, r domain.Reply) error { s.last = r; return nil }

// stubTodo 는 ListTasks 만 답하는 fake TodoistPort 다.
type stubTodo struct{ tasks []todoist.Task }

func (s stubTodo) ListTasks(ctx context.Context, filter string) ([]todoist.Task, error) {
	return s.tasks, nil
}
func (s stubTodo) AddTask(ctx context.Context, c, d, p string) (todoist.Task, error) {
	return todoist.Task{}, nil
}
func (s stubTodo) CompleteTask(ctx context.Context, id string) error          { return nil }
func (s stubTodo) UpdateTask(ctx context.Context, id, c, d string) error      { return nil }
func (s stubTodo) DeleteTask(ctx context.Context, id string) error            { return nil }

func TestMorningBriefing_WithCalendar(t *testing.T) {
	t.Parallel()
	todo := stubTodo{tasks: []todoist.Task{{Content: "운동"}}}
	cal := &fakeCalPort{events: []gcal.Event{{Summary: "스탠드업", Start: time.Now()}}}
	sender := &capSender{}
	job := NewMorningBriefing(todo, cal, sender, "C1")
	job(context.Background())
	if !strings.Contains(sender.last.Text, "스탠드업") || !strings.Contains(sender.last.Text, "운동") {
		t.Fatalf("브리핑에 일정+할일 둘 다 없음:\n%s", sender.last.Text)
	}
}

func TestMorningBriefing_NilCalendar(t *testing.T) {
	t.Parallel()
	todo := stubTodo{tasks: []todoist.Task{{Content: "운동"}}}
	sender := &capSender{}
	job := NewMorningBriefing(todo, nil, sender, "C1")
	job(context.Background())
	if !strings.Contains(sender.last.Text, "운동") {
		t.Fatalf("할일 브리핑 누락:\n%s", sender.last.Text)
	}
}
```
> 주의: `stubTodo` 가 구현해야 하는 `TodoistPort` 메서드 집합은 실제 인터페이스(`internal/agent/todoist_port.go`)를 보고 정확히 맞춘다(위는 예시 — 시그니처 다르면 맞춰 수정). 기존 `todoist_briefing_test.go` 에 이미 fake 가 있으면 그것을 재사용한다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestMorningBriefing -v`
Expected: FAIL (시그니처 불일치 / 컴파일 에러).

- [ ] **Step 3: 구현**

`internal/agent/todoist_briefing.go` 의 `NewMorningBriefing` 을 교체:
```go
// NewMorningBriefing 은 아침 브리핑 작업을 만든다(오늘 일정 + 오늘·밀린 할일).
// cal 이 nil 이면 할일만(기존 동작).
func NewMorningBriefing(port TodoistPort, cal CalendarPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		var sections []string
		if evLines, ok := todayEventLines(ctx, cal); ok {
			sections = append(sections, "📅 *오늘 일정*\n"+evLines)
		}
		tasks, err := port.ListTasks(ctx, "today | overdue")
		if err != nil {
			log.FromContext(ctx).Error("아침 브리핑 조회 실패", "error", err)
		} else if len(tasks) > 0 {
			sections = append(sections, "☀️ *오늘 할 일과 밀린 일*\n"+formatTaskLines(tasks))
		}
		if len(sections) == 0 {
			sendText(ctx, sender, channel, "☀️ 오늘 마감할 일이 없습니다. 좋은 하루 보내세요!")
			return
		}
		sendText(ctx, sender, channel, strings.Join(sections, "\n\n"))
	}
}

// todayEventLines 는 오늘 일정을 • 줄로 만든다. cal nil/오류면 ("", false)(best-effort).
func todayEventLines(ctx context.Context, cal CalendarPort) (string, bool) {
	if cal == nil {
		return "", false
	}
	mn, mx := eventRange(calNow(), "today")
	evs, err := cal.ListEvents(ctx, mn, mx)
	if err != nil {
		log.FromContext(ctx).Error("브리핑 일정 조회 실패", "error", err)
		return "", false
	}
	if len(evs) == 0 {
		return "", false
	}
	return formatEvents(evs), true
}
```
- import 에 `"strings"` 가 없으면 추가. `log` 는 이미 import 되어 있음(기존 파일 사용 중).
- `cmd/server/main.go` 의 morning 브리핑 등록부를 임시로 nil 전달로 고쳐 컴파일 유지:
  ```go
  Fn: agent.NewMorningBriefing(todoistClient, nil, sender, cfg.TodoistBriefingChannel)})
  ```
  (Task 9 에서 nil → calPort 로 교체.)

- [ ] **Step 4: 테스트 통과 + 빌드 확인**

Run: `cd ~/personal-agent/jarvis && go test ./internal/agent/ -run TestMorningBriefing -v && go build ./...`
Expected: PASS + 빌드 성공.

- [ ] **Step 5: 커밋**

```bash
cd ~/personal-agent/jarvis
git add internal/agent/todoist_briefing.go internal/agent/calendar_briefing_test.go cmd/server/main.go
git commit -m "feat(agent): 아침 브리핑에 오늘 일정 섹션 통합(cal nil 안전)"
```

---

### Task 9: main 배선 + 최종 검증

**Files:**
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `gcal.New` (Task 2), `agent.CalendarTools`/`agent.NewCalendarApplier`/`agent.CalendarSystemHint`/`agent.DefaultSystemPrompt` (Tasks 4-6), `Config.Google*` (Task 7), `NewMorningBriefing` (Task 8).

- [ ] **Step 1: 캘린더 클라이언트 구성 + 도구/applier/프롬프트 배선**

`cmd/server/main.go` import 에 `"github.com/Jongseong0111/jarvis/internal/gcal"` 추가.

`tools` 구성과 `applier` 구성 사이(또는 적절한 위치)에 캘린더 활성화 블록 추가. `ctx` 는 main 의 컨텍스트(없으면 `context.Background()`):
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
```

`applier` 의 DispatchApplier 맵에 `delete_event` 를 추가한다. 현재 코드:
```go
	var applier domain.ProposalApplier = agent.NewHomeApplier(home, renderer)
	if todoistClient != nil {
		applier = agent.NewDispatchApplier(
			map[string]domain.ProposalApplier{"delete_todo": agent.NewTodoistApplier(todoistClient)},
			applier,
		)
	}
```
를, 캘린더가 활성일 때 `delete_event` 도 포함하도록 맵을 구성하게 수정. 두 기능 독립적으로 합쳐지도록 맵을 먼저 만들고 비어있지 않으면 DispatchApplier 로 감싼다:
```go
	var applier domain.ProposalApplier = agent.NewHomeApplier(home, renderer)
	byOp := map[string]domain.ProposalApplier{}
	if todoistClient != nil {
		byOp["delete_todo"] = agent.NewTodoistApplier(todoistClient)
	}
	if calPort != nil {
		byOp["delete_event"] = agent.NewCalendarApplier(calPort)
	}
	if len(byOp) > 0 {
		applier = agent.NewDispatchApplier(byOp, applier)
	}
```

`agent.New` 호출의 시스템 프롬프트를, 캘린더 활성 시 힌트를 덧붙여 전달:
```go
	sysPrompt := agent.DefaultSystemPrompt
	if calPort != nil {
		sysPrompt += agent.CalendarSystemHint
	}
	ag := agent.New(geminiClient, visionClient, tools, sysPrompt)
```

- [ ] **Step 2: 아침 브리핑에 calPort 전달**

주의: morning 브리핑 등록은 main 이 아니라 별도 함수 `startBriefings(ctx, cfg, todoistClient, geminiClient, sender, logger)`(cmd/server/main.go) **안**에 있다. 따라서 `calPort` 를 그 함수로 넘겨야 한다:
1. `startBriefings` 시그니처에 `calPort agent.CalendarPort` 파라미터 추가(예: `sender` 뒤).
2. main 에서 `startBriefings(...)` 호출부에 `calPort` 인자 추가.
3. `startBriefings` 안의 morning 등록부를 Task 8 의 `nil` 에서 교체:
   ```go
   	Fn: agent.NewMorningBriefing(todoistClient, calPort, sender, cfg.TodoistBriefingChannel)})
   ```
   (evening/digest 브리핑은 그대로.)

- [ ] **Step 3: 빌드 + vet + 전체 테스트**

Run:
```bash
cd ~/personal-agent/jarvis && go build ./... && go vet ./... && go test ./... -race 2>&1 | tail -20
```
Expected: 전부 green(기존 + 신규).

- [ ] **Step 4: 커밋**

```bash
cd ~/personal-agent/jarvis
git add cmd/server/main.go
git commit -m "feat(server): 캘린더 도구/applier/프롬프트/브리핑 배선"
```

- [ ] **Step 5: 수동 검증(컨트롤러/사용자, 코드 외)**

> 코드 작업 아님 — 서브에이전트는 건너뛰고 컨트롤러가 안내. 순서:
> 1. GCP: 프로젝트 → Calendar API 활성화 → OAuth 동의화면 구성 → **Desktop app** OAuth 클라이언트 생성 → client_id/secret 획득.
> 2. `config/.env` 에 `GOOGLE_OAUTH_CLIENT_ID`/`GOOGLE_OAUTH_CLIENT_SECRET` 기입.
> 3. `go run ./cmd/calauth` → 브라우저 로그인/동의 → 출력된 `GOOGLE_CALENDAR_REFRESH_TOKEN` 을 `.env` 에 추가.
> 4. 서버 재빌드·재기동(`go build -o bin/jarvis ./cmd/server` → 재실행).
> 5. 슬랙 라이브: "이번주 일정 알려줘"(list), "내일 오후 3시 치과 예약 추가해줘"(add 즉시), "치과 언제였지?"(search), "치과 예약 삭제해줘"(버튼 승인). 아침 브리핑에 일정 섹션 확인.

---

## 자체 리뷰 결과(작성자 체크)

- **Spec 커버리지:** §3.1 gcal Event/auth → Task 1; §3.1 client → Task 2; §3.2 calauth → Task 3; §3.8 현재시각/힌트 → Task 4; §3.3 도구 → Task 5; §3.4 applier → Task 6; §3.6 config → Task 7; §3.5 브리핑 → Task 8; §3.7 배선 → Task 9. §2 결정(SDK/즉시add/버튼delete/토글) 전부 매핑.
- **플레이스홀더:** SDK 시그니처는 모듈 캐시에서 실측 확인(NewService/option/Events 빌더/EventDateTime/oauth2.Config). calauth 의 AuthCodeURL 옵션은 본문 주석으로 명시. TBD 없음.
- **타입 일관성:** `CalendarPort`(4 메서드) → tools/applier/briefing/main 에서 동일 사용. `ChangeProposal{Op:"delete_event", Fields:{event_id,summary}}` → applier 가 같은 키로 읽음. `eventRange`/`parseEventTimes`/`formatEvents` → tools·briefing 공유. `calNow` 패키지 변수로 정적 시간 주입.
- **컴파일 무결성:** Task 8 이 `NewMorningBriefing` 시그니처를 바꾸며 main 호출부를 nil 로 동시 수정(green 유지), Task 9 가 nil→calPort 로 마무리.

## 범위 밖 (YAGNI)
저녁 브리핑 캘린더 통합, update_event(수정), 다중 캘린더/참석자/알림, 반복 일정 생성, "급한 일" 전용 도구.
