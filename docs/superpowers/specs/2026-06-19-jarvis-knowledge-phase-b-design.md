# Jarvis 지식 Phase B — Claude Code 세션 다리로 개념화·리뷰·PR 설계

작성일: 2026-06-19

## 1. 배경 / 목표

Phase A(완료): jarvis가 ChatGPT 공유링크 → 요약 → `knowledge-base/sources/conversation/`에 저장.
Phase B: 그 소스를 **개념 문서로 정리**한다 — 단, 단순 자동화가 아니라 **슬랙에서 대화로 리뷰·수정·추가·삭제**한 뒤 승인하면 커밋 + 자동 PR.

개념화 엔진은 방금 재설계·검증한 `/kb-ingest`(+`/kb-approve`/`/kb-reject`) 스킬(knowledge-base 레포). **jarvis는 이 스킬을 돌리는 Claude Code 세션과 슬랙 사이의 얇은 다리**다.

> 의존: 호스트에 `claude` CLI + `gh` CLI 설치. kb 레포는 개인 GitHub(`Jongseong0111`).

## 2. 핵심 아키텍처 — jarvis = stateful Claude Code 세션의 슬랙 다리

```
Slack ── jarvis ──┬─ (평소) Gemini 에이전트 (집정리·요약 등)
                  └─ (리뷰 모드) Claude Code 세션 (kb 레포에서 ingest/수정/approve)
```

- jarvis가 kb 레포에서 `claude -p --output-format json`으로 ingest를 시작 → 응답에서 **session_id** 캡처.
- 이후 큐레이션 메시지는 `claude -p --resume <session_id>`로 **같은 세션에 이어붙임** → 개념 수정·추가·삭제가 전부 Claude Code 능력으로.
- 채널이 **"kb 리뷰 모드"**인 동안 그 채널 메시지는 Gemini 에이전트 대신 Claude 세션으로 라우팅. 승인/취소 시 모드 종료.
- Claude의 **최종 메시지를 jarvis가 슬랙에 그대로 게시**(jarvis는 파싱 안 함). 출력 형식은 프롬프트로 지시(트리 + 개념별 1줄).

## 3. 흐름

```
[발동]
- (a) Phase A "저장해" 직후 → 에이전트가 "개념으로 정리할까?" 제안 → "응"
- (b) 명시적: "@jarvis 이 소스 개념정리해줘" (최근/지정 소스)

1. jarvis: kb 레포에서
     git checkout -b kb/ingest-<slug>
     claude -p --output-format json --permission-mode acceptEdits \
       "/kb-ingest sources/conversation/<...>.md --type=conversation
        끝나면 제안 개념 트리 + 각 개념 1줄 요약을 슬랙용으로 정리해서 보여줘."
   → session_id 저장, 채널을 리뷰 모드로. 결과(트리+미리보기) 슬랙 게시.
   (백그라운드 실행, "정리 중…" 먼저)

2. [리뷰 모드] 사용자 큐레이션 메시지 →
     claude -p --resume <session_id> "<메시지>" --output-format json
   예) "channel 빼", "goroutine에 Y 추가", "Z 개념 새로 만들어", "go/overview 더 짧게"
   → Claude가 draft 파일 수정 → 변경 요약 슬랙 게시. (반복)

3. [승인] "승인/이대로 가자" →
     claude -p --resume <session_id>
       "/kb-approve drafts/pending/<...> 한 뒤, 이 브랜치를 push 하고 gh 로 main 대상 PR 생성해줘."
   → 커밋 + push + PR 생성 → PR 링크 슬랙 게시. 리뷰 모드 종료.

   [취소] "취소/그만" → /kb-reject + 브랜치 폐기. 모드 종료.
```

## 4. 컴포넌트 (jarvis 측, 구현은 Todoist 작업 종료 후)

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `internal/claudecode/runner.go` | `claude -p` 실행 래퍼: 최초 실행(세션 생성) + `--resume`. `--output-format json` 파싱으로 `{session_id, result}` 추출. ctx 타임아웃(예: 5분). | 신규 |
| `internal/agent/review_session.go` | 채널별 리뷰 세션 레지스트리(channel→{session_id, branch, source, slug, busy}). 모드 진입/종료. | 신규 |
| `internal/slack/handler.go` | 메시지 라우팅 분기: 채널이 리뷰 모드면 Claude 세션 다리로, 아니면 기존 Gemini 에이전트로. | 변경 |
| `internal/agent/knowledge_tools.go` | 도구 `start_concept_ingest(source_path?)` 추가(쓰기형). Phase A 저장 후 제안 + 명시 요청 둘 다 진입점. | 변경 |
| `pkg/config/config.go` | `KNOWLEDGE_REPO_PATH`(있음) 재사용. `gh`/`claude` 경로는 PATH 전제. | (영향 없음) |
| `cmd/server/main.go` | review 브리지 + runner 조립. | 변경 |

> jarvis가 직접 git/gh를 호출하지 않고 **Claude 세션에 지시**(흐름 3)하는 쪽을 기본으로 한다 — kb 작업(approve/branch/push/PR)이 한 세션·한 레포 안에서 일관되게 일어나고, jarvis는 메시지 중계만. (대안: jarvis가 git/gh 직접 — 더 통제적이나 이중 관리. 1차는 Claude 위임.)

## 5. 비동기 / 동시성

- 매 `claude -p` 호출은 느림(웹검색 포함 수십초~수 분). → **goroutine**으로 실행, 먼저 "🧠 정리 중…" 게시, 완료 시 결과 게시.
- **채널당 직렬**: 리뷰 세션의 `busy` 플래그로 한 번에 한 claude 호출만. 진행 중 새 메시지는 "아직 처리 중이야, 잠깐만" 또는 큐.
- 세션 TTL: 일정 시간(예: 30분) 무응답이면 리뷰 모드 자동 해제(브랜치는 남김 — 사용자가 나중에).

## 6. 권한 / 안전

- 헤드리스 `claude -p`는 비대화형이라 권한 프롬프트에 막히면 안 됨 → `--permission-mode acceptEdits` (+ kb 레포 작업/`git`/`gh`/WebSearch 허용). ⚠️ **자율 실행**이라 인지 필요하나, 작업 디렉터리가 kb 레포로 한정되고 결과는 PR로 한 번 더 사람 리뷰를 거침.
- 안전 원칙(CLAUDE.md): 승인 전 main 커밋/머지 금지 → **충족**: ingest는 feature 브랜치, 승인=PR 생성까지, **머지는 사람이 GitHub에서**(git diff 최종 리뷰).
- push/PR은 개인 GitHub 계정으로(kb 레포의 includeIf/SSH 그대로).

## 7. 에러 처리

- claude CLI 실패/타임아웃 → 로그 + "개념 정리 중 문제가 생겼어. 다시 시도할까?" + 리뷰 모드 해제(브랜치 정리 여부 안내).
- session_id 유실/만료 → 새 세션으로 재시작 안내.
- `gh` 미설치/인증 실패 → 커밋·push까지는 되되 PR 단계에서 "PR은 수동으로: <링크>" 안내.
- 빈 ingest(소스에서 개념 0) → "정리할 개념을 못 찾았어".

## 8. 결과 표시 형식 (슬랙)

Claude의 최종 메시지를 그대로 게시. 프롬프트로 아래 형식 지시:
```
🗂️ 개념 정리 제안 (브랜치 kb/ingest-<slug>)

language/go/
  • overview — Go 개요(강점·비교)
  • goroutine — 경량 스레드, 자동 CPU 양보
  • channel — 락 없는 고루틴 통신
ai/
  • harness-engineering — Agent=Model+Harness
  ...
드롭/언급: java 비교, dive-coding …

수정할 거 있으면 말해줘 (예: "channel 빼", "X 추가"). 좋으면 "승인".
```

## 9. 테스트 전략

- `internal/claudecode/runner.go`: `claude` 호출은 외부 프로세스 → **fake 실행기 인터페이스**로 단위 테스트(주입). 실제 claude는 라이브 검증. JSON 파싱(session_id/result 추출)은 픽스처로 단위 테스트.
- `internal/agent/review_session.go`: 모드 진입/종료/busy 직렬화 단위 테스트(순수 상태).
- `internal/slack/handler.go`: 리뷰 모드면 브리지로, 아니면 에이전트로 라우팅되는지 fake로.
- 라이브: 실제 소스 → ingest → "channel 빼" → "승인" → PR 링크 확인.

## 10. 범위

**In:** jarvis가 Claude Code 세션을 다리로 ingest/리뷰/수정/승인→커밋+PR. 발동(Phase A 연계 + 명시), 리뷰 모드 라우팅, 비동기, 브랜치+PR.

**Out:**
- `/kb-ingest` 스킬 자체(Phase A에서 재설계·검증 완료).
- Todoist/스케줄러(별도, 현재 다른 작업 진행 중).
- 다중 채널 동시 ingest의 정교한 큐잉(1차는 채널당 직렬).
- 슬랙 버튼식 큐레이션(대화형으로 충분 — 버튼은 향후).

## 11. 구현 시점

jarvis에서 Todoist 작업이 진행 중 → **이 설계는 문서만**. 구현(Go 코드)은 그 작업 종료/머지 후 깨끗한 jarvis에서 시작.
