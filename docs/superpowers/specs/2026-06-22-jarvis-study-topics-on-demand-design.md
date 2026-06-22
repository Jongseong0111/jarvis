# 대화형 공부 주제 재생성 설계

날짜: 2026-06-22
상태: 확정

## 개요

아침 digest와 별개로, 사용자가 Slack 대화로 공부 주제를 다시/다르게 받을 수 있게 한다.
예: "다른 공부 주제 줄래?", "운영체제 주제 줘", "DB 다른 거".

jarvis 에이전트(Gemini function-calling)에 읽기 도구 `suggest_study_topics` 를 추가하고,
기존 `devdigest` 패키지의 도메인/주제 생성 로직을 재사용한다.

## 기능 범위

### 포함
- `suggest_study_topics(domain?)` 도구: 공부 주제 3-5개 생성
  - domain 미지정 → 11개 도메인 중 모델이 하나 선택
  - domain 지정 → 그 도메인(또는 세부 주제 힌트)으로 생성
- 한 대화 내 "방금 거 말고 또"는 에이전트의 채널 대화 기억이 처리(직전 응답을 참고해 다르게 생성)

### 제외 (의도적)
- 아침 digest 내용 저장/연동 — **무상태**. 에이전트는 아침에 뭘 보냈는지 모른다.
  따라서 "(아침과) 같은 도메인"은 사용자가 도메인을 직접 말해야 한다.
- 제외 주제 명시 인자 없음

## 아키텍처

```
사용자: "운영체제 다른 공부 주제 줘" / "다른 거 줄래?"
  ↓
jarvis agent (Gemini function-calling 루프)
  ↓ suggest_study_topics(domain?) 호출 (읽기 도구 = 즉시 Run)
  ↓
devdigest.GeminiGenerator.GenerateTopics(ctx, domain) — Gemini 1 call
  ↓
포맷된 공부 주제 텍스트 반환 → 에이전트가 Slack 응답으로 전달
```

## 파일 구조

| 파일 | 역할 |
|---|---|
| `internal/devdigest/digest.go` | `TopicResult` 타입 + `GenerateTopics` 메서드 + `buildTopicPrompt` 추가 |
| `internal/devdigest/digest_test.go` | `buildTopicPrompt` 분기 테스트 |
| `internal/agent/study_tools.go` | `StudyTopicGenerator` 포트 + `StudyTools` + `suggest_study_topics` Run 도구 |
| `internal/agent/study_tools_test.go` | fake generator로 포맷·domain 인자 검증 |
| `cmd/server/main.go` | 도구 배선(스터디 생성기 생성 + tools 에 append) |
| `internal/agent/agent.go` | `DefaultSystemPrompt` 에 도구 사용 힌트 1줄 추가 |

## devdigest 변경

```go
// TopicResult 는 공부 주제 생성 결과다.
type TopicResult struct {
    Domain string
    Topics []string
}

// GenerateTopics 는 공부 주제만 생성한다.
// requestedDomain 이 비면 모델이 11개 도메인 중 하나를 선택하고,
// 지정되면 그 도메인(또는 더 구체적인 세부 주제 힌트)으로 계층형 주제를 만든다.
func (g *GeminiGenerator) GenerateTopics(ctx context.Context, requestedDomain string) (TopicResult, error)
```

- 응답 파싱은 기존 `parseResponse` 재사용 (Domain+Topics 만 사용, news 는 빔).
- `systemPrompt`, `domains` 상수/변수 그대로 재사용.
- `buildTopicPrompt(requestedDomain string)`:
  - requestedDomain == "" → "아래 도메인 중 하나 선택: <11개>" + 계층형 지시
  - requestedDomain != "" → "도메인/주제: <requestedDomain> 에 대해" + 계층형 지시
  - 두 경우 모두 출력 JSON: `{"domain":"...","topics":["..."]}`

## agent 도구

```go
// StudyTopicGenerator 는 공부 주제를 생성하는 능력이다(테스트에서 fake 주입).
type StudyTopicGenerator interface {
    GenerateTopics(ctx context.Context, domain string) (devdigest.TopicResult, error)
}

// StudyTools 는 공부 주제 도구 목록을 만든다.
func StudyTools(gen StudyTopicGenerator) []Tool
```

도구 선언:
- 이름: `suggest_study_topics`
- 설명: "개발 공부 주제를 추천한다. 사용자가 '다른 공부 주제', '운영체제 주제', 'DB 다른 거' 등을 요청할 때 호출. domain 인자에 특정 도메인을 넣으면 그 주제로, 비우면 임의 도메인으로 생성한다."
- 파라미터: `domain`(string, optional) — "공부 도메인 또는 세부 주제. 예: 운영체제, 데이터베이스, 쿠버네티스. 미지정 시 임의 선택."
- Run: GenerateTopics 호출 → 포맷 문자열 반환

반환 포맷(아침 digest의 공부주제 섹션과 동일):
```
📚 *공부 주제*  _(도메인: 운영체제)_
• 운영체제 → 스케줄링 → CFS vs EEVDF
• 운영체제 → 메모리 관리 → 페이지 폴트 처리
• 운영체제 → 동기화 → 뮤텍스 vs 세마포어
```

## 시스템 프롬프트 추가 (1줄)

`DefaultSystemPrompt` 핵심 규칙에 추가:
> - 사용자가 "공부 주제 추천/다른 거/특정 도메인(운영체제 등) 주제"를 요청하면 suggest_study_topics 를 호출한다. 특정 도메인을 말하면 domain 인자에 넣고, 아니면 비운다.

## 배선 (main.go)

```go
studyGen := devdigest.NewGenerator(geminiClient)
tools = append(tools, agent.StudyTools(studyGen)...)
```

- 기존 `geminiClient` 재사용. digest용 generator(startBriefings 내부)와 별개 인스턴스지만 같은 클라이언트라 무방.
- Todoist 활성 여부와 무관하게 항상 등록(읽기 도구라 부작용 없음).

## 에러 처리

- Gemini 실패 → 도구가 error 반환 → 에이전트 루프가 사용자에게 실패를 짧게 알림(기존 패턴).
- 빈 topics → 도구가 그대로 포맷(드묾). 모델이 최소 3개를 생성하도록 프롬프트에서 유도.

## 테스트 전략

- `devdigest`: `buildTopicPrompt` 분기 테스트(도메인 지정 시 그 문자열 포함, 미지정 시 11개 도메인 목록 포함). `GenerateTopics` 는 기존 방침대로 순수 함수 위주(파싱은 `parseResponse` 기존 테스트가 커버).
- `agent`: `study_tools_test.go` — fake `StudyTopicGenerator` 로
  - domain 인자가 generator 에 전달되는지
  - 반환 텍스트에 도메인·주제가 포함되는지
  - generator error 시 도구가 error 반환하는지

## 도메인 목록 (기존 재사용)

```go
// devdigest.domains
언어 / 웹·백엔드 / 데이터베이스 / 인프라 / 데이터 /
운영체제 / 네트워크 / 자료구조·알고리즘 / 개발도구 / AI / 기타
```
