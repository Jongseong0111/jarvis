# Jarvis — 개인 통합 에이전트 서버 (구조 재편 + Phase 1) 설계

작성일: 2026-06-15

## 1. 목표

`base/CLAUDE.md`가 정의한 "자연어 입력 → 여러 개인 시스템을 대신 조작하는 개인 운영 에이전트"를
현재 구현 상태에 맞춰 재편하고, 첫 실행 가능한 단위(Phase 1: Slack echo)를 구현한다.
코딩/구조 컨벤션은 회사 서버 `acloset-api`의 것을 규모에 맞게 차용한다(§4).

이번 작업의 범위:

1. 디렉터리/레포 구조를 현재 구현 기준으로 재편 (knowledge-base 분리, jarvis 신설)
2. 두 레포를 개인 GitHub 계정으로만 동작하도록 git identity 스코핑
3. jarvis에 acloset-api 컨벤션 베이스라인 셋업 (레이아웃/린트/테스트/Makefile)
4. Phase 1: Slack 메시지를 받아 echo 응답하는 jarvis 서버 구현

명시적으로 이번 범위가 **아닌** 것: Intent Router, Notion 연동, 지식저장소 Worker 연동(Phase 2~5).

## 2. 목표 디렉터리 구조

모든 개인 프로젝트를 회사 작업공간(`~/acloset-agent/`)에서 분리해 `~/personal-agent/`로 이동한다.
`~/personal-agent/`는 git repo가 아닌 단순 작업공간 폴더이며, 그 안에 독립 repo 둘을 sibling으로 둔다.

```txt
~/personal-agent/                       # 작업공간 폴더 (git repo 아님)
  jarvis/                               # Go 에이전트 서버 (신규 git repo, 개인계정)
    cmd/server/main.go                  # 진입점 + DI 조립
    domain/                             # 인터페이스/DTO/모델만 (구현 없음)
      slack.go                          # Slack 채널 인터페이스 + Incoming/Reply DTO
    internal/
      slack/                            # Slack 채널 어댑터 (acloset-api의 route/http 대응)
        client.go                       # socketmode 연결/이벤트 루프
        handler.go                      # 이벤트 → echo 변환 → 응답
        handler_test.go
    pkg/
      config/config.go                  # Config struct + New() + 검증
      log/log.go                        # slog 기반 구조화 로거
    config/
      .env.example                      # 로컬 토큰 템플릿
    Makefile                            # run/test/lint/mock 타깃
    .golangci.yaml                      # 린트 설정 (acloset-api 축약본)
    .mockery.yaml                       # mock 생성 설정 (후속 Phase 대비)
    .gitignore                          # .env, 빌드 산출물 제외
    go.mod                              # module github.com/Jongseong0111/jarvis
    CLAUDE.md                           # base/CLAUDE.md 에서 이동 + 갱신
    docs/superpowers/{specs,plans}/     # jarvis 스펙/플랜 (이 문서 포함)
  knowledge-base/                       # 기존 repo 통째 이동 (개인계정)
    docs/specs/                         # base/docs의 KB 설계/플랜 이동
    (기존 concepts/ sources/ drafts/ maps/ _schemas/ 유지)
```

이동 매핑:

| 현재 위치 | 이동 위치 |
|---|---|
| `~/acloset-agent/base/CLAUDE.md` | `~/personal-agent/jarvis/CLAUDE.md` (갱신) |
| `~/acloset-agent/base/docs/superpowers/specs/*` (KB 설계) | `~/personal-agent/knowledge-base/docs/specs/` |
| `~/acloset-agent/base/docs/superpowers/plans/*` (KB MVP 플랜) | `~/personal-agent/knowledge-base/docs/specs/` |
| `~/acloset-agent/base/knowledge-base/` | `~/personal-agent/knowledge-base/` |
| 이 spec 문서 | `~/personal-agent/jarvis/docs/superpowers/specs/` |

이동 후 `~/acloset-agent/base/`는 비워지므로 삭제한다.

## 3. Git identity 스코핑 (개인계정 전용)

회사 global git 계정과 물리적으로 분리하고, `~/personal-agent/` 아래 모든 repo가
**자동으로 개인계정 + 개인 SSH 키만** 사용하도록 한다.

값:
- GitHub username: `Jongseong0111`
- commit email: `leon1111@naver.com`
- 개인 SSH 키: 신규 생성 `~/.ssh/id_ed25519_personal` (공개키는 사용자가 GitHub에 등록)

방식 — `includeIf`로 경로 스코핑. `~/.gitconfig`(global)에 추가:
```ini
[includeIf "gitdir:~/personal-agent/"]
    path = ~/.gitconfig-personal
```

`~/.gitconfig-personal` (신규):
```ini
[user]
    name = Jongseong0111
    email = leon1111@naver.com
[core]
    sshCommand = ssh -i ~/.ssh/id_ed25519_personal -o IdentitiesOnly=yes
```

효과:
- `~/personal-agent/` 아래 repo의 commit 작성자는 무조건 개인계정.
- push 시 `IdentitiesOnly=yes`로 개인 SSH 키만 사용 → 회사 키와 충돌/혼선 없음.
- remote URL은 평범한 `git@github.com:Jongseong0111/<repo>.git` 사용 가능 (host alias 불필요).

SSH 키 생성:
```bash
ssh-keygen -t ed25519 -C "leon1111@naver.com" -f ~/.ssh/id_ed25519_personal -N ""
```
생성 후 `~/.ssh/id_ed25519_personal.pub` 내용을 사용자가 GitHub → Settings → SSH and GPG keys 에 등록.
원격 repo(`jarvis`, `knowledge-base`)는 사용자가 GitHub에서 직접 생성한다.

## 4. 컨벤션 (acloset-api 차용)

acloset-api는 Echo/GORM 기반 대형 HTTP 서버다. jarvis는 작은 로컬 Slack 워커이므로
**패턴/철학은 그대로 차용하고, 무거운 인프라(APM·Secrets Manager·codegen·testcontainers)는 규모에 맞게 적응**한다.

### 4.1 그대로 차용

- **Clean Architecture 계층 + 경계**: `Domain → (Store) → Usecase/Worker → Channel`.
  - `domain/`: 인터페이스/DTO/모델 **정의만**, 구현 금지.
  - 채널(Slack) 어댑터는 acloset-api의 `route/http` 위치에 대응 → `internal/slack/`.
  - 비즈니스 로직은 Usecase/Worker에. (Phase 1 echo엔 Worker가 아직 없음 → 후속 Phase에서 추가)
- **디렉터리 분리**: `cmd/`, `domain/`, `internal/`, `pkg/`, `config/`.
- **Constructor 패턴**: `func New(deps...) domain.Xxx { return &xxx{...} }` — private struct, 인터페이스 반환, 의존성은 인터페이스로 주입. global mutable state 금지. DI는 `main.go`에서 수동 조립.
- **Value receiver 기본** (mutate/non-copyable/측정된 성능 이슈에서만 포인터).
- **에러 철학**: 비즈니스 판단 에러와 인프라 실패 에러를 구분, 컨텍스트와 함께 래핑, **double-wrap 금지**(이미 감싼 에러는 as-is 전파).
- **네이밍**: 파일 `snake_case`, 인터페이스 `{Domain}Store`/`{Domain}Usecase`, `time.Time` 필드는 `At` 접미사, 변수 축약 금지(`request` O, `req` X), JSON 태그 camelCase·약자는 대문자(`userID`, `imageURL`).
- **테스트**: table-driven, `t.Parallel()` 첫 줄, **정적 시간 주입**(`time.Now()` 금지), mock은 `.EXPECT()` 체인만(`.On()`/`MatchedBy` 금지).
- **한국어**: 모든 주석/커밋 메시지/PR은 한국어, 기술 식별자는 영어(`ctx`, `goroutine`).
- **워크플로우**: `make run/test/lint/mock`, `golangci-lint` 통과 기준.

### 4.2 규모에 맞게 적응 (acloset-api와 의도적으로 다름)

- **로깅**: acloset-api는 `zerolog + Elastic APM + logrus 호환`. jarvis는 APM/logrus 레거시가 없는 그린필드라
  **stdlib `log/slog`**를 쓴다. 단 사용 형태(구조화 필드, 컨텍스트 인지 로거 `log.FromContext(ctx)`)는 동일하게 맞춘다.
- **Config**: acloset-api는 `YAML + AWS Secrets Manager`. jarvis는 개인 로컬 서버이므로 **`.env`/환경변수**로 로드
  (CLAUDE.md "API key는 .env 또는 keychain" 원칙). 단 `pkg/config`의 `Config` struct + `New()` + 필수값 검증 + 누락 시 즉시 종료 패턴은 유지. (로컬 편의를 위해 `godotenv`로 `.env` 자동 로드)
- **에러**: acloset-api의 `pkg/errors` 코드젠(`error.yaml`/`make errorgen`)과 HTTP status 매핑은 보류.
  stdlib `errors` + `fmt.Errorf("...: %w", err)` 사용, 필요해지면 작은 커스텀 에러 타입 도입. 채널이 Slack이라 HTTP status 매핑 불필요.

### 4.3 보류/후속 (Phase 1 미적용)

- `exhaustruct` 등 엄격 린터는 초기엔 제외(과함). `errcheck`, `ineffassign`, `paralleltest`, `wrapcheck`, `govet`, `staticcheck` 정도의 축약본으로 시작.
- `testcontainers`(DB 없음), Elastic APM, Echo/GORM 계층은 해당 Phase에서 도입.
- `mockery`는 설정만 두고(`.mockery.yaml`), 실제 mock 대상 인터페이스는 Worker가 생기는 Phase 2+에서.

## 5. Phase 1 에이전트 서버 설계 (Slack echo)

### 연결 방식: Socket Mode

로컬 맥에서 public URL 없이 동작. jarvis가 Slack으로 나가는 WebSocket을 열어 이벤트를 수신한다.
인바운드 네트워킹/터널/TLS 불필요. `CLAUDE.md`의 "클라우드 전제 안 함" 원칙과 일치.

- 라이브러리: `github.com/slack-go/slack` + `slack-go/slack/socketmode`
- 토큰: `SLACK_BOT_TOKEN`(xoxb-), `SLACK_APP_TOKEN`(xapp-, Socket Mode용 app-level token)

### 동작 흐름

```txt
Slack (app_mention 또는 DM)
  → socketmode 이벤트 수신 (internal/slack/client.go)
  → log.FromContext(ctx) 기록
  → handler가 echo 변환 (buildEcho, 순수함수)
  → 같은 채널로 응답
```

처리 대상은 **app_mention 이벤트와 DM(im) 메시지로 한정**한다.
봇 자신의 메시지·일반 채널 메시지는 무시 → echo 무한루프 원천 차단.

### 컴포넌트

| 파일 | 책임 | 의존 |
|---|---|---|
| `cmd/server/main.go` | 조립: config 로드 → logger → slack client 생성 → 이벤트 루프 실행 | pkg/config, pkg/log, internal/slack |
| `pkg/config/config.go` | `.env`/환경변수 로드, `Config` struct, 필수 토큰 검증(없으면 즉시 종료) | godotenv |
| `pkg/log/log.go` | slog 기반 구조화 로거 + `FromContext(ctx)` | - |
| `domain/slack.go` | 채널 인터페이스(`MessageSender`) + `IncomingMessage`/`Reply` DTO | - |
| `internal/slack/client.go` | socketmode 연결/이벤트 루프, slack-go 이벤트 → `IncomingMessage` 변환 | slack-go, domain |
| `internal/slack/handler.go` | `IncomingMessage` → `Reply` 결정, 전송 | domain, pkg/log |
| `internal/slack/handler_test.go` | `buildEcho` 단위테스트 (table-driven) | - |

순수함수 경계 (테스트 핵심):
```go
// buildEcho 는 수신 메시지로부터 응답을 계산한다. SDK/네트워크 비의존 → 단위테스트 가능.
func buildEcho(in domain.IncomingMessage) (reply domain.Reply, ok bool)
```
- 멘션 텍스트에서 봇 멘션 토큰(`<@U...>`)을 제거한 나머지를 echo.
- 빈 텍스트면 `ok=false`(응답 안 함).

### 설정 (config/.env.example)

Phase 1에 필요한 것만:
```env
SLACK_BOT_TOKEN=
SLACK_APP_TOKEN=
```
(`OPENAI_API_KEY`, `NOTION_*` 등은 후속 Phase에서 추가.)

## 6. 테스트 전략

- `buildEcho` 등 순수 변환 로직은 TDD + table-driven + `t.Parallel()`로 단위테스트(`handler_test.go`).
- `pkg/config` 로드/검증 단위테스트(필수 토큰 누락 시 에러).
- socketmode 연결 루프는 실제 Slack 연동이라 수동 검증:
  Slack 워크스페이스에서 봇 멘션 → 동일 텍스트 echo 확인.

## 7. CLAUDE.md 업데이트

`CLAUDE.md`를 jarvis로 이동하면서 현재 구현 기준으로 갱신한다:
- "권장 디렉터리 구조" 섹션을 실제 구조(`~/personal-agent/jarvis` + sibling `knowledge-base`)로 교체.
- 컨벤션이 acloset-api 차용임을 명시(§4 요지).
- 지식 저장소가 **이미 Claude Code skills 기반으로 구현**되어 있음을 반영(`/kb-ingest`, `/kb-approve` 등 — knowledge-base/README.md 참조).
- `KNOWLEDGE_REPO_PATH`를 `~/personal-agent/knowledge-base`로 명시.
- Phase 구분/안전 원칙 등 설계 의도는 유지.

## 8. 향후 범위 (이번 제외)

- Phase 2: Intent Router (LLM intent 분류 → enum) + Worker 인터페이스 도입(여기서 mockery 본격 사용)
- Phase 3: 집 정리 Notion 연동 + 승인 플로우
- Phase 4: 지식저장소 Worker ↔ 기존 knowledge-base repo 연동 (Claude Code headless 실행)
- Phase 5: Todo / 스케줄러 / Web UI / 카카오톡 / 음성

## 9. 결정 로그

- KB는 jarvis와 **완전 분리된 독립 repo**로 유지 (승인 후 commit/push 플로우의 핵심).
- 모든 개인 프로젝트를 회사 작업공간 밖 `~/personal-agent/`로 이동.
- 첫 구현은 Phase 1(Slack echo)만. 이후 피드백 반영하며 점진 확장.
- 스택 Go 확정. 컨벤션은 acloset-api 차용(§4), 무거운 인프라는 적응/보류.
- 로깅은 acloset-api(zerolog+APM)와 달리 **slog** 채택 확정(그린필드, APM 불필요).
- Config는 YAML+SecretsManager 대신 **.env/환경변수**(개인 로컬 서버).
- echo 대상은 멘션 + DM (무한루프 방지).
