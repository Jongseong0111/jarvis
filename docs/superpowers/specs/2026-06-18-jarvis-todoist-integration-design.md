# Jarvis — Todoist 양방향 연동 (할일 + 스케줄 브리핑) 설계

작성일: 2026-06-18

## 1. 배경 / 목표

CLAUDE.md Phase 5의 **Todo 시스템 + 스케줄러**를 Todoist로 구현한다. 원래 "Notion Todo DB" 후보였으나, Todoist는 이미 모바일/데스크톱 앱·리마인더·반복일정을 잘 갖춘 전용 할일 앱이므로 저장소로 채택한다.

핵심 통찰 두 가지가 설계를 결정한다:

1. **마감 알림은 Todoist가 직접 보낸다.** 그래서 jarvis inbound의 가치는 "마감 다시 알리기"가 아니라 *Todoist 상태를 Slack 허브로 끌어오는 것*(아침/저녁 브리핑)이다.
2. **jarvis는 로컬 우선**(맥에서 Slack Socket Mode)이라 공개 HTTP 엔드포인트가 없다. Todoist **webhook은 공개 HTTPS + OAuth 앱 등록이 필수**라 이 아키텍처와 충돌한다. 따라서 inbound는 **주기 폴링(스케줄 브리핑)**으로 구현하고, 마침 로드맵에 있던 **SchedulerWorker를 여기서 시작**한다.

"양방향"의 의미:
- **Outbound (jarvis → Todoist):** 자연어로 할일 추가/조회/완료/수정/삭제.
- **Inbound (Todoist → jarvis):** 스케줄러가 주기적으로 Todoist를 읽어 아침/저녁 브리핑을 Slack에 전송.

## 2. 범위

**In:**
- Todoist REST thin 클라이언트(`internal/todoist`) — Add/List/Complete/Update/Delete.
- 에이전트 도구 5종: `add_todo` `list_todos` `complete_todo` `update_todo`(즉시 실행), `delete_todo`(변경안 → 승인 버튼).
- 인프로세스 스케줄러(`internal/scheduler`) — "매일 HH:MM에 작업" 등록·실행.
- 아침/저녁 브리핑 2종 — Todoist를 읽어 한국어 요약을 정해진 Slack 채널로 전송.
- 설정: `TODOIST_API_TOKEN`(있으면 기능 on, 없으면 graceful off), 브리핑 채널·시각·타임존.

**Out (이번 범위 아님):**
- Todoist webhook / 실시간 푸시(공개 엔드포인트·OAuth 필요).
- OAuth 멀티유저(개인 토큰 1개로 충분).
- 고정 2회를 넘는 임의 cron 스케줄, 사용자별 브리핑 커스터마이즈.
- 프로젝트/라벨 관리, 코멘트, 협업 기능.

## 3. 인증 / API

- **인증:** 개인용 단일 사용자 → **personal API token** 1개. Todoist 설정 → Integrations → Developer 에서 발급. `Authorization: Bearer <token>`. OAuth 불필요.
- **API:** Todoist 통합 API(`https://api.todoist.com/api/v1`)를 대상으로 한다. (구 REST v2 `…/rest/v2`는 deprecated.) 정확한 엔드포인트·필드는 구현 시 라이브로 검증한다(네트워크 호출 라이브 검증 컨벤션).
  - 할일 목록/필터: 필터 쿼리로 `today`, `overdue`, `tomorrow` 등 사용.
  - 완료: 해당 태스크 close.
  - 추가/수정/삭제: 태스크 CRUD.
- **선택 기능:** `TODOIST_API_TOKEN`이 비어 있으면 도구·스케줄러를 등록하지 않고 로그만 남긴다(Notion 맵 페이지가 선택인 것과 동일 패턴). 필수 검증(`config.validate`)에 추가하지 않는다.

## 4. 컴포넌트

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `internal/todoist/types.go` | `Task`/`Project` DTO, 필터 상수 | 신규 |
| `internal/todoist/client.go` | `AddTask/ListTasks(filter)/CompleteTask(id)/UpdateTask(id,…)/DeleteTask(id)` — `net/http` thin 클라이언트(`notion/client.go` 미러) | 신규 |
| `internal/agent/todoist_port.go` | `TodoistPort` 인터페이스(테스트 fake 주입) | 신규 |
| `internal/agent/todoist_tools.go` | `TodoistTools(port) []Tool` — add/list/complete/update(Run) + delete(Propose) | 신규 |
| `internal/agent/todoist_briefing.go` | 태스크 목록 → 한국어 브리핑 텍스트 조립(순수 함수) | 신규 |
| `internal/scheduler/scheduler.go` | `Job{Name,Hour,Min,TZ,Fn}`, `Register`, `Run(ctx)` — 매일 HH:MM goroutine 루프 | 신규 |
| `internal/agent/applier.go` | `NewTodoistApplier`(delete_todo 처리) + `NewDispatchApplier`(Op 분기) 추가 | 변경 |
| `internal/agent/agent.go` | system 프롬프트에 Todoist 사용 지침 추가 | 변경 |
| `pkg/config/config.go` | Todoist 설정 5종 로드 | 변경 |
| `cmd/server/main.go` | Todoist 클라이언트·도구·스케줄러 조립, 승인 applier 합성 | 변경 |

집정리/지식 도구는 무변경(도구 목록에 append).

## 5. 클라이언트 계약 (`internal/todoist`)

```go
type Task struct {
    ID      string
    Content string
    Due     string // 표시용 문자열(예: "2026-06-18" 또는 "오늘"), 없으면 ""
    Project string
    URL     string // 앱에서 열 링크(있으면)
}

type Client struct{ /* http.Client, token */ }

func New(token string) *Client
func (c *Client) ListTasks(ctx context.Context, filter string) ([]Task, error)
func (c *Client) AddTask(ctx context.Context, content, due, project string) (Task, error)
func (c *Client) CompleteTask(ctx context.Context, id string) error
func (c *Client) UpdateTask(ctx context.Context, id, content, due string) error
func (c *Client) DeleteTask(ctx context.Context, id string) error
```

- `filter`는 Todoist 필터 문법(예: `today | overdue`, `tomorrow`)을 그대로 전달.
- 에러는 `fmt.Errorf("…: %w", err)`로 감싸고, non-2xx는 본문 일부를 포함한 에러로.

## 6. 에이전트 도구 (`internal/agent/todoist_tools.go`)

**승인 정책:** `add`/`complete`/`update`는 **즉시 실행(Run)**, `delete`만 **변경안(Propose) → 승인 버튼**.
- 근거: CLAUDE.md 안전원칙은 *삭제·대량 변경*만 승인 필수로 규정한다. 집정리(add도 승인)와 달리 할일 추가/완료는 가볍고 빈번하며 가역적이라(Todoist 앱에서 즉시 되돌림 가능) 마찰을 줄인다. 삭제는 규칙대로 승인.

| 도구 | 종류 | 동작 |
|---|---|---|
| `list_todos(filter?)` | Run | 기본 `today \| overdue`. 태스크를 `• 내용 — 마감 (id)` 형태로 반환. |
| `add_todo(content, due?, project?)` | Run | `AddTask` → "추가했어: <내용> (<마감>)". `due`는 자연어 그대로 전달(Todoist가 파싱) 또는 날짜. |
| `complete_todo(query)` | Run | `query`로 미완료 태스크 1개 resolve → `CompleteTask`. 0개/다수면 에이전트가 되묻기(`resolveTask` = home의 `resolveItem` 미러). |
| `update_todo(query, due?, content?)` | Run | resolve 후 `UpdateTask`. 바꿀 값 없으면 에러. |
| `delete_todo(query)` | Propose | resolve 후 `ChangeProposal{Op:"delete_todo", Fields:{task_id, content}}` 생성 → 승인 버튼. |

`resolveTask(ctx, query)`: `ListTasks`로 내용 부분일치 검색 → 1개면 반환, 0개 "못 찾았어", 다수면 후보 나열 후 되묻기.

system 프롬프트 추가(요지):
- "할일/투두 관련 요청은 Todoist 도구를 써라. 추가/완료/수정은 바로 실행하고 결과를 짧게 알려라."
- "완료/수정/삭제는 먼저 내용으로 태스크를 찾는다. 모호하면 되묻는다."
- "삭제는 `delete_todo`로 변경안을 만들고 승인 버튼을 거친다(바로 지우지 않는다)."

## 7. 승인 처리 (`delete_todo`)

기존 `InteractionHandler`는 applier 1개(`HomeApplier`)만 받는다. Todoist 삭제 승인을 위해 `cmd/server/main.go`에서 **Op 접두로 분기하는 합성 applier**를 조립한다:

```go
// home.* 와 delete_todo 를 각각의 applier 로 라우팅
applier := agent.NewDispatchApplier(map[string]domain.ProposalApplier{
    "delete_todo": agent.NewTodoistApplier(todoistClient),
    // 그 외(home ops)는 기본 HomeApplier 로
}, homeApplier)
```

- `TodoistApplier.Apply`는 `Op=="delete_todo"`일 때 `DeleteTask(Fields["task_id"])` 호출 후 "삭제했어: <내용>" Reply 반환.
- `DispatchApplier`는 `p.Op`로 applier를 고르고, 없으면 기본(home)으로 위임 → 기존 home 승인 플로우 무변경.

## 8. 스케줄러 (`internal/scheduler`)

```go
type Job struct {
    Name string
    Hour int
    Min  int
    TZ   *time.Location
    Fn   func(ctx context.Context)
}

type Scheduler struct{ jobs []Job }
func New() *Scheduler
func (s *Scheduler) Register(j Job)
func (s *Scheduler) Run(ctx context.Context) // 각 Job 마다 goroutine
```

- 각 Job: `nextFire(now, hour, min, tz)`로 다음 발화 시각 계산(오늘 HH:MM 지났으면 내일) → `time.NewTimer` → 발화 시 `Fn`을 **recover + 개별 타임아웃(예: 30s)**으로 실행 → 다음 발화 재계산 반복. `ctx.Done()`이면 종료.
- `nextFire`는 **순수 함수**(now·tz 주입)로 단위 테스트.
- 재시작 시 타이머 리셋되지만 다음 발화를 다시 계산하므로 무해(상태 비저장).
- 로드맵의 SchedulerWorker가 이 패키지로 성장한다(나중에 작업 늘면 `robfig/cron`으로 교체 가능).

## 9. 브리핑 (`internal/agent/todoist_briefing.go`)

스케줄러가 호출하는 두 함수. 각각 Todoist를 읽어 텍스트를 만들고 `MessageSender.Send`로 정해진 채널에 보낸다.

- **아침(기본 08:00 Asia/Seoul):** 필터 `today | overdue` → "☀️ 오늘 할일" + 밀린 항목 강조. 할일 없으면 "오늘 마감 할일 없어 👍" 전송.
- **저녁(기본 21:00):** 필터 `today & !checked` + `tomorrow` → "🌙 오늘 미완료 / 내일 할일". **미완료·내일 둘 다 없으면 전송 생략**(조용).
- 본문 조립부 `formatBriefing(title string, tasks []todoist.Task) string`는 순수 함수(태스크 슬라이스 → 텍스트)로 분리해 단위 테스트.
- 전송 실패는 로그로 남기고 사용자 흐름을 막지 않는다(브리핑은 보조 기능).

## 10. 설정 (`pkg/config`)

| 변수 | 필수 | 기본값 | 설명 |
|---|---|---|---|
| `TODOIST_API_TOKEN` | | (없음) | 비면 Todoist 기능 전체 off |
| `TODOIST_BRIEFING_CHANNEL` | | (없음) | 브리핑 보낼 Slack 채널/DM ID. 비면 브리핑 off(도구는 on) |
| `TODOIST_MORNING_TIME` | | `08:00` | 아침 브리핑 시각 `HH:MM` |
| `TODOIST_EVENING_TIME` | | `21:00` | 저녁 브리핑 시각 `HH:MM` |
| `TODOIST_BRIEFING_TZ` | | `Asia/Seoul` | 브리핑 타임존 |

- `HH:MM` 파싱·타임존 로드 실패 시 설정 에러로 즉시 실패(조용한 오작동 방지).
- README 환경변수 표 + 로드맵 항목 갱신.

## 11. DI 조립 (`cmd/server/main.go`)

```text
if cfg.TodoistAPIToken == "" → 로그 "Todoist 비활성" 후 기존대로
else:
  client := todoist.New(token)
  tools = append(tools, agent.TodoistTools(client)...)
  applier = DispatchApplier{delete_todo→TodoistApplier, else→HomeApplier}
  if BriefingChannel != "":
     sched := scheduler.New()
     sched.Register(아침 Job{MorningTime, TZ, fn=아침브리핑(client, sender, channel)})
     sched.Register(저녁 Job{EveningTime, TZ, fn=저녁브리핑(...)})
     go sched.Run(ctx)
```

## 12. 에러 처리 / 안전

- 도구 실패(네트워크·404 등) → 짧은 한국어 안내("할일 추가 중 문제가 생겼어").
- `resolveTask` 0/다수 → 에이전트가 되묻기.
- 삭제는 승인 버튼 필수(안전원칙). 추가/완료/수정은 가역적이라 즉시 실행.
- 토큰은 `.env`/환경변수만(하드코딩 금지). 모든 작업 로그.
- 브리핑 실패는 사용자 대화 흐름과 격리(로그만).

## 13. 테스트 전략

- `internal/todoist/client.go`: 네트워크라 **라이브 검증**(기존 컨벤션). 가능하면 `httptest` 서버로 요청 조립(헤더/바디/경로)·응답 파싱 단위 테스트.
- `internal/agent/todoist_tools.go`: fake `TodoistPort` → 각 도구가 올바른 포트 호출·반환, `delete_todo`가 `ChangeProposal` 생성, `resolveTask` 0/1/다수 분기.
- `internal/agent/todoist_briefing.go`: `formatBriefing`에 정적 태스크 슬라이스 주입 → 텍스트·빈 목록 분기 검증.
- `internal/scheduler`: `nextFire`에 정적 now·tz 주입 → 오늘/내일 경계, 타임존 검증(table-driven, `t.Parallel()`).
- 전 패키지 `go test/vet/build` green, `gofmt -l` 빈 출력.

## 14. 완료 기준 (수동/라이브)

Slack에서:
- "@jarvis 오늘 Clone Graph 다시 풀기 추가해줘" → Todoist에 등록 + "추가했어" 응답, 앱에 보임.
- "오늘 할일 뭐야?" → 오늘+밀린 목록.
- "Clone Graph 끝났어" → 해당 할일 완료 처리.
- "그 할일 삭제해줘" → 승인 버튼 → 승인 시 삭제.
- 아침/저녁 지정 시각에 브리핑 DM 도착(없는 날 아침은 "할일 없어", 저녁은 무음).
- `TODOIST_API_TOKEN` 미설정 시 기존 동작 회귀 없음(도구·스케줄러 미등록).
