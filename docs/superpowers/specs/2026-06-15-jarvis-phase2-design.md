# Jarvis — Phase 2: Intent Router 설계

작성일: 2026-06-15

## 1. 목표

Phase 1(Slack echo)에 이어, 수신 메시지를 **의도(intent)로 분류**하고 해당 **Worker로 디스패치**하는
라우팅 계층을 만든다. 실제 외부 시스템 연동(Notion=Phase 3, 지식저장소 Claude Code=Phase 4)은
이번 범위가 **아니다** — Worker는 인터페이스 + 스텁으로만 둔다.

이번 작업의 범위:

1. `Intent` enum 타입과 도메인 인터페이스(`Classifier`, `Worker`, `MessageRouter`) 정의
2. Gemini API 기반 `Classifier` 구현 (자연어 → 엄격한 enum)
3. namespace 기준 Worker 디스패치 라우터 구현
4. home / knowledge / system Worker **스텁** 구현
5. Slack `Handler`를 echo에서 라우터 위임으로 리팩터
6. config에 Gemini 설정 추가, `main.go` 조립

명시적으로 이번 범위가 **아닌** 것: 실제 Notion 연동, 지식저장소 Claude Code 연동, todo/scheduler intent,
confidence 점수, 범용 LLM 클라이언트 추출(Phase 3에서 home Worker가 LLM을 쓸 때).

## 2. 데이터 흐름

```txt
Slack mention/DM
  ↓
slack.Handler
  - 멘션 토큰 제거 (Slack 전용 정리, Phase 1 로직 유지)
  - 빈 텍스트면 무시
  ↓
router.Route(ctx, in)
  - Classifier.Classify(text) → Intent          [Gemini structured output]
  - workers[intent.Namespace()] 선택 (없으면 system 워커)
  - Worker.Handle(ctx, intent, in) → Reply
  ↓
sender.Send (Slack 응답)
```

**경계 원칙**: Slack 고유 처리(멘션 토큰 제거)는 채널 어댑터(`internal/slack`)에 남기고,
의도 판단·디스패치는 `internal/router`로 분리한다. 도메인/라우터는 채널 독립적인 평문 텍스트만 다룬다.

## 3. 도메인 타입 (`domain/`)

새 파일 `domain/router.go`:

```go
// Intent 는 메시지가 어떤 작업을 의도하는지 나타내는 분류 결과다.
type Intent string

const (
	IntentHomeSearch      Intent = "home.search"
	IntentHomeAdd         Intent = "home.add"
	IntentHomeUpdate      Intent = "home.update"
	IntentHomeDelete      Intent = "home.delete"
	IntentKnowledgeUpdate Intent = "knowledge.update"
	IntentKnowledgeSearch Intent = "knowledge.search"
	IntentKnowledgeReview Intent = "knowledge.review"
	IntentSystemHelp      Intent = "system.help"
	IntentUnknown         Intent = "system.unknown"
)

// Namespace 는 intent 의 네임스페이스("home"/"knowledge"/"system")를 반환한다.
func (i Intent) Namespace() string

// Classifier 는 평문 텍스트를 Intent 로 분류하는 능력이다.
type Classifier interface {
	Classify(ctx context.Context, text string) (Intent, error)
}

// Worker 는 분류된 메시지를 처리해 Reply 를 생성하는 능력이다.
type Worker interface {
	Handle(ctx context.Context, intent Intent, in IncomingMessage) (Reply, error)
}

// MessageRouter 는 수신 메시지를 분류·디스패치해 Reply 를 만드는 능력이다.
type MessageRouter interface {
	Route(ctx context.Context, in IncomingMessage) (Reply, error)
}
```

Phase 2 enum 범위는 위 9개로 고정한다. todo/scheduler는 대응 Worker가 없으므로 분류 대상에서 제외한다
(분류 프롬프트에도 노출하지 않는다).

`AllIntents()` 같은 유효 intent 목록 헬퍼를 제공해, Gemini enum 제약과 검증에서 공용으로 쓴다.

## 4. 컴포넌트

| 파일 | 책임 |
|---|---|
| `domain/router.go` | 위 타입/인터페이스 + `Namespace()`, `AllIntents()` |
| `internal/router/router.go` | `MessageRouter` 구현. classify → `intent.Namespace()`로 Worker 선택 → 위임. 매핑 없으면 system Worker |
| `internal/router/gemini_classifier.go` | `Classifier` 구현. Gemini structured output(enum 제약)으로 유효 enum 보장. 호출 실패는 error 반환, 비유효 응답은 `system.unknown` |
| `internal/workers/home.go` | 스텁. 예: "집 정리 작업으로 인식했어 (home.add). 아직 Notion 연동은 준비 중이야." |
| `internal/workers/knowledge.go` | 스텁. 동일 패턴 |
| `internal/workers/system.go` | `system.help`=기능 안내 텍스트, `system.unknown`=되묻기("무슨 작업인지 잘 모르겠어. 집 정리/지식 저장소 중 뭐에 관한 거야?") |
| `internal/slack/handler.go` | echo 제거 → `domain.MessageRouter` 의존으로 리팩터 (멘션 제거·빈 텍스트 무시는 유지) |
| `pkg/config/config.go` | `GEMINI_API_KEY`(필수), `GEMINI_MODEL`(기본 `gemini-2.5-flash`) 추가 |
| `cmd/server/main.go` | classifier + workers(map) + router 조립, handler에 주입 |

### 4.1 Router 디스패치

`Router`는 `workers map[string]domain.Worker`(키=namespace)와 fallback용 system Worker를 갖는다.

```txt
intent := classifier.Classify(ctx, in.Text)
worker := workers[intent.Namespace()]   // 없으면 systemWorker
return worker.Handle(ctx, intent, in)
```

home Worker는 home.* 전부를, knowledge Worker는 knowledge.* 전부를, system Worker는 system.* 전부를
sub-action 스위치로 처리한다(Phase 2 스텁이라 분기 메시지만 다름).

### 4.2 Gemini 분류

- SDK: `google.golang.org/genai`(공식 Go SDK). enum responseSchema를 지원해 출력을 유효 intent로 제약.
- 프롬프트: 시스템 지시 + intent 목록/설명 + 사용자 텍스트. 출력은 단일 enum 값.
- 모호하면 `system.unknown`을 고르도록 프롬프트에 명시(스펙의 "잘 모르겠으면 unknown" 반영).
- 방어선: SDK가 enum 밖 값을 줄 가능성에 대비해 `AllIntents()`로 검증, 미지의 값이면 `system.unknown`으로 흡수.
- `Classifier` 인터페이스 뒤라, 향후 `claude -p` 기반 구현으로 드롭인 교체 가능.

## 5. 에러 처리 (CLAUDE.md 안전 원칙 반영)

- **분류 호출 실패**: 전체 에러를 `slog`로 로그 + 사용자에겐 짧게("처리 중 문제가 생겼어, 잠시 후 다시 시도해줘").
  `handler.go`가 라우터 error를 받아 이 짧은 응답으로 변환.
- **비유효/저신뢰 분류**: `system.unknown`으로 흡수 → 되묻기.
- 모든 분류 결과(intent)와 실패는 로그로 남긴다.

## 6. 테스트 전략

acloset-api 차용 컨벤션: table-driven + `t.Parallel()` + 정적 시간, SDK/네트워크 비의존 순수 로직 우선.

- `internal/router/router_test.go`: fake `Classifier`/`Worker`로 — intent별 올바른 Worker 선택, namespace 매핑 없을 때 system 폴백, Classifier error 전파.
- `internal/router/gemini_classifier_test.go`: 순수 부분만 — 응답 문자열 → Intent 검증 함수(미지의 값 → `system.unknown`), 프롬프트 빌더. 실제 API 호출은 얇게 유지(단위 테스트 대상 외).
- `internal/workers/*_test.go`: 각 Worker가 sub-intent별 기대 Reply 텍스트를 만드는지.
- `internal/slack/handler_test.go`: mock `domain.MessageRouter`(mockery)로 — 멘션 제거, 빈 메시지 무시, 정상 라우팅 시 Send 호출, 라우터 error → 에러 응답 Send.

`.mockery.yaml`에 `domain.MessageRouter`(및 필요 시 `Classifier`/`Worker`) 추가.

## 7. 설정 변경

`config/.env`(및 `.env.example`)에 추가:

```env
GEMINI_API_KEY=        # 필수
GEMINI_MODEL=gemini-2.5-flash   # 선택, 기본값 존재
```

`config.validate()`에 `GEMINI_API_KEY` 빈 값 검사 추가.

## 8. 완료 기준 (수동 검증)

서버 실행 후 Slack에서:

- `@jarvis 건전지 어디 뒀지?` → home 스텁 응답(`home.search` 인식 표시)
- `@jarvis TLS 개념 정리해서 저장해줘` → knowledge 스텁 응답(`knowledge.update` 인식 표시)
- `@jarvis 도와줘` → system.help 안내
- `@jarvis 오늘 날씨 어때?` → `system.unknown` 되묻기

각 응답이 짧고 명확한지(응답 스타일 원칙) 확인.

## 9. YAGNI로 자른 것

- confidence 점수/임계값 (LLM이 직접 unknown 판정)
- todo/scheduler intent (Phase 5)
- 범용 LLM 클라이언트 추출 (Phase 3에서 home Worker가 LLM 쓸 때 공통화)
- 대화 컨텍스트/멀티턴 (단발 분류만)
