# jarvis

자연어로 개인 시스템(집 정리·지식 저장소 등)을 대신 조회/수정/정리해주는 **개인 운영 에이전트 서버**.
슬랙에 한 줄 보내면 → jarvis가 의도를 파악해 적절한 도구를 실행하고 → 결과를 다시 슬랙으로 돌려준다.

> Go로 작성된 개인 프로젝트. 입력 채널은 슬랙(추후 카카오톡/음성), LLM은 Gemini.
> 설계 철학과 상세 규칙은 [`CLAUDE.md`](./CLAUDE.md) 참고.

## 무엇을 하나

- **집 정리** — "안방 수납장1에 체온계 넣었어" → Notion에 등록 제안 → 승인 → 반영 + "우리집 지도" 페이지 자동 갱신. "체온계 어디 있어?" → 위치 검색.
- **사진 → 물건 판별** — 집 사진을 멘션과 함께 올리면 비전 모델이 물건을 인식해 카테고리별 등록 제안.
- **지식 저장소 (Phase A)** — ChatGPT 공유 링크를 보내면 대화를 요약해 보여주고, 슬랙에서 대화로 다듬은 뒤 "저장해"하면 `knowledge-base` 레포에 기록.

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
외부 시스템: Notion(집정리) · knowledge-base 레포(지식)
```

**안전 원칙**: LLM은 자연어 해석과 변경안 생성만 한다. 실제 외부 시스템 수정은 제한된 서버 코드로만 수행하고, 삭제·대량 변경은 승인 버튼을 거친다.

## 디렉터리

```
cmd/server/main.go      진입점 + DI 조립
domain/                 채널 독립 인터페이스/DTO (slack.go, router.go)
internal/
  slack/                Slack 어댑터 (client, handler, interaction)
  agent/                도구 가진 LLM 에이전트 (agent, home_tools, knowledge_tools, applier, mapview, memory)
  gemini/               Gemini 호출 (client, vision, generate)
  notion/               Notion REST thin 클라이언트
  knowledge/            ChatGPT 공유링크 추출·요약·저장
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

## 개발

```bash
go test ./...      # 전체 테스트
go vet ./...
gofmt -l .         # 포맷 점검 (빈 출력 = 정렬됨)
```

- Clean Architecture(Domain→Worker→Channel) + 생성자 주입 + value receiver + table-driven 테스트(`t.Parallel()`) + 한국어 주석/커밋.
- 네트워크 호출(Gemini, HTTP)은 라이브 검증, 순수 로직은 단위 테스트.

## 로드맵

- **지식저장소 Phase B** — 저장된 요약을 Claude Code headless로 개념별 문서로 분리 → 슬랙 승인 → git 커밋.
- 할일 관리, 스케줄러(주기 알림), ChatGPT 기록 정리(OpenAI 연동).
- 입력 채널 확장: 카카오톡, 음성.
