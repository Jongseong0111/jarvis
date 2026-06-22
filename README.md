# jarvis

자연어로 개인 시스템(집 정리·지식 저장소 등)을 대신 조회/수정/정리해주는 **개인 운영 에이전트 서버**.
슬랙에 한 줄 보내면 → jarvis가 의도를 파악해 적절한 도구를 실행하고 → 결과를 다시 슬랙으로 돌려준다.

> Go로 작성된 개인 프로젝트. 입력 채널은 슬랙(추후 카카오톡/음성), LLM은 Gemini.
> 설계 철학과 상세 규칙은 [`CLAUDE.md`](./CLAUDE.md) 참고.

## 무엇을 하나

- **집 정리** — "안방 수납장1에 체온계 넣었어" → Notion에 등록 제안 → 승인 → 반영 + "우리집 지도" 페이지 자동 갱신. "체온계 어디 있어?" → 위치 검색.
- **사진 → 물건 판별** — 집 사진을 멘션과 함께 올리면 비전 모델이 물건을 인식해 카테고리별 등록 제안.
- **지식 저장소** — ChatGPT 공유 링크를 보내면 대화를 요약·다듬어 `knowledge-base` 레포에 저장하고(Phase A), 이어서 "개념 정리해줘"하면 Claude Code headless가 저장된 소스를 개념별 문서로 분리·생성 → 슬랙 승인 → git 커밋(Phase B).
- **할일 (Todoist)** — "오늘 Clone Graph 풀기 추가해줘"/"오늘 할일 뭐야?"/"끝났어"로 추가·조회·완료·수정, 삭제는 승인. 아침/저녁 스케줄 브리핑.
- **일정 (Google Calendar)** — "이번주 일정 알려줘"/"내일 3시 치과 추가해줘"/"아기 검진 언제였지?"로 조회·추가·검색, 삭제는 승인. "급한 일?"엔 캘린더+할일을 합쳐 답하고, 아침 브리핑에 오늘 일정 포함.
- **개발 다이제스트** — 매일 아침 GeekNews·HN 등에서 개발 뉴스 + 공부 주제를 요약해 브리핑. "다른 주제 추천해줘"로 대화형 재생성.
- **LLM 비용 추적** — 모든 Gemini/Claude 호출 비용을 `~/.jarvis/usage.jsonl`에 자동 기록. "오늘 비용 얼마야?"로 source/model/기능별 집계 조회.

## 구조

```
Slack (Socket Mode)
   │  app_mention / DM
   ▼
jarvis 서버 (Go)
   │
   ▼
LLM 에이전트 (Gemini function-calling 루프, 채널별 대화 기억)
   ├─ 잡담/질문      → 자연어로 응답
   └─ 작업 의도      → 도구(tool) 호출
        ├─ 읽기 도구  → 즉시 실행 후 답변
        └─ 쓰기 도구  → 변경안 생성 → 슬랙 승인 버튼 → 반영
   ▼
외부 시스템: Notion(집정리) · knowledge-base 레포(지식) · Todoist(할일) · Google Calendar(일정)
```

**안전 원칙**: LLM은 자연어 해석과 변경안 생성만 한다. 실제 외부 시스템 수정은 제한된 서버 코드로만 수행하고, 삭제·대량 변경은 승인 버튼을 거친다.

> 모든 LLM 호출 비용은 `internal/usage`가 호출 단위로 JSONL에 자동 기록한다(에이전트 흐름과 무관한 best-effort).

## 디렉터리

```
cmd/server/main.go      진입점 + DI 조립
cmd/calauth/main.go     Google Calendar OAuth refresh token 1회 발급 CLI
domain/                 채널 독립 인터페이스/DTO (slack.go, router.go)
internal/
  slack/                Slack 어댑터 (client, handler, interaction)
  agent/                도구 가진 LLM 에이전트 (agent, home/knowledge/todoist/calendar/study/usage tools, applier, mapview, memory, briefing)
  gemini/               Gemini 호출 (client, vision, generate) + 비용 sink
  notion/               Notion REST thin 클라이언트
  todoist/              Todoist REST thin 클라이언트
  gcal/                 Google Calendar 클라이언트 (공식 SDK + OAuth2)
  knowledge/            ChatGPT 공유링크 추출·요약·저장
  devdigest/            개발 뉴스·공부 주제 다이제스트 생성
  claudecode/           Claude Code CLI 실행 (지식 ingest) + 비용 sink
  scheduler/            매일 HH:MM 인프로세스 스케줄러
  usage/                LLM 비용 JSONL 기록·집계
pkg/
  config/               .env/환경변수 로드·검증
  log/                  slog 구조화 로거
docs/superpowers/       설계(specs)·구현계획(plans) 문서
config/.env             토큰 (gitignore)
```

## 설정

`config/.env` (gitignore) 또는 환경변수로 주입한다.

| 변수 | 필수 | 설명 |
|---|---|---|
| `SLACK_BOT_TOKEN` | ✅ | 봇 토큰 (`xoxb-...`). 사진 첨부 다운로드엔 `files:read` scope 필요 |
| `SLACK_APP_TOKEN` | ✅ | 앱 레벨 토큰 (`xapp-...`, Socket Mode) |
| `GEMINI_API_KEY` | ✅ | Gemini API 키 |
| `GEMINI_MODEL` | | 에이전트 로직 모델 (기본 `gemini-2.5-flash`) |
| `GEMINI_VISION_MODEL` | | 사진 판별 모델 (기본 `gemini-2.5-flash-lite`) |
| `NOTION_API_KEY` | ✅ | Notion integration 토큰 |
| `NOTION_LOCATIONS_DB_ID` | ✅ | 장소 DB |
| `NOTION_CATEGORIES_DB_ID` | ✅ | 카테고리 DB |
| `NOTION_ITEMS_DB_ID` | ✅ | 물건 DB |
| `NOTION_HOME_URL` | | 집정리 페이지 링크(안내용) |
| `NOTION_MAP_PAGE_ID` | | "우리집 지도" 자동 렌더 페이지 |
| `KNOWLEDGE_REPO_PATH` | | 지식저장소 경로 (기본 `~/personal-agent/knowledge-base`) |
| `TODOIST_API_TOKEN` | | Todoist 개인 토큰. 없으면 할일 기능 off |
| `TODOIST_BRIEFING_CHANNEL` | | 브리핑 보낼 Slack 채널/DM ID. 없으면 브리핑 off |
| `TODOIST_MORNING_TIME` | | 아침 브리핑 시각(기본 `08:00`) |
| `TODOIST_EVENING_TIME` | | 저녁 브리핑 시각(기본 `21:00`) |
| `TODOIST_BRIEFING_TZ` | | 브리핑 타임존(기본 `Asia/Seoul`) |
| `DIGEST_TIME` | | 개발 다이제스트 브리핑 시각(기본 `09:00`) |
| `DIGEST_RSS_URLS` | | 추가 RSS 피드 URL(쉼표 구분) |
| `GOOGLE_OAUTH_CLIENT_ID` | | Google OAuth 클라이언트 ID(Desktop 앱). 없으면 캘린더 off |
| `GOOGLE_OAUTH_CLIENT_SECRET` | | Google OAuth 클라이언트 시크릿 |
| `GOOGLE_CALENDAR_REFRESH_TOKEN` | | `cmd/calauth`로 발급. 비면 캘린더 기능 off |
| `GOOGLE_CALENDAR_ID` | | 대상 캘린더(기본 `primary`) |
| `USAGE_LOG_PATH` | | LLM 비용 기록 JSONL 경로(기본 `~/.jarvis/usage.jsonl`) |

## 실행

```bash
go build -o bin/jarvis ./cmd/server
./bin/jarvis
```

백그라운드(세션 독립)로 띄우려면:

```bash
nohup ./bin/jarvis > /tmp/jarvis.log 2>&1 &
tail -f /tmp/jarvis.log          # 로그
pkill -f bin/jarvis              # 종료
```

> 맥 잠자기/종료 시 멈춘다. 계속 켜두려면 `caffeinate -dimsu &`.

### Google Calendar 1회 셋업

캘린더 기능을 쓰려면 refresh token이 필요하다(최초 1회).

1. GCP에서 **Calendar API 활성화** + **OAuth 클라이언트(Desktop 앱)** 생성 → client ID/secret을 `config/.env`에 기입.
2. 동의 화면은 **게시(production)** 권장(테스팅이면 refresh token 7일 만료).
3. 토큰 발급:
   ```bash
   go run ./cmd/calauth     # 출력된 URL을 브라우저에서 열어 동의 → refresh token 출력
   ```
4. 출력된 `GOOGLE_CALENDAR_REFRESH_TOKEN=...`을 `config/.env`에 추가하고 서버 재기동.

## 개발

```bash
go test ./...      # 전체 테스트
go vet ./...
gofmt -l .         # 포맷 점검 (빈 출력 = 정렬됨)
```

- Clean Architecture(Domain→Worker→Channel) + 생성자 주입 + value receiver + table-driven 테스트(`t.Parallel()`) + 한국어 주석/커밋.
- 네트워크 호출(Gemini, HTTP)은 라이브 검증, 순수 로직은 단위 테스트.

## 로드맵

완료:
- ✅ 집 정리(Notion) + 사진 물건 판별
- ✅ 지식 저장소 Phase A(ChatGPT 공유링크 요약·저장) + Phase B(Claude Code 개념 정리 → 승인 → git)
- ✅ 할일(Todoist) + 아침/저녁 브리핑 스케줄러
- ✅ 일정(Google Calendar) 조회/추가/검색/삭제 + 아침 브리핑
- ✅ 개발 다이제스트(뉴스·공부 주제) 아침 브리핑
- ✅ LLM 비용 추적(JSONL + `list_usage`)

예정:
- 입력 채널 확장: 카카오톡, 음성.
