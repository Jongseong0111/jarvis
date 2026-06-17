# Jarvis — 사진 → 물건 판별 (집정리 비전 입력) 설계

작성일: 2026-06-17

## 1. 배경 / 목표

집정리에 물건을 넣을 때 일일이 이름을 타이핑하는 대신, **사진을 찍어 올리면 물건을 자동 인식**해서 등록안을 만들어준다.

```txt
Slack: @jarvis 안방 수납장1에 정리함  [사진 첨부]
  → 사진 속 물건 자동 인식 → 카테고리별 add_items 변경안 → 승인 → Notion 반영
```

핵심 원칙: **비전은 "사진에 뭐가 있나"만 뽑고, 나머지(장소 resolve · 카테고리 지정 · 변경안 · 승인)는 기존 에이전트를 그대로 재사용**한다. 새로 만드는 표면을 최소화한다.

이번 범위: Slack 이미지 첨부 → 물건 인식 → 기존 add_items 흐름. 범위 밖: 사진에서 위치 자동 추론, 여러 장 일괄 다른 장소, 멀티턴 기억.

## 2. 모델 분리 (비용/안정성)

| 용도 | 모델 | 이유 |
|---|---|---|
| 에이전트 루프(함수호출) | flash (`GEMINI_MODEL`) | 툴 호출 안정성. flash-lite 는 함수호출 약함 |
| 사진 물건 판별(비전) | flash-lite (`GEMINI_VISION_MODEL`) | 단순 이미지→JSON 추출. 함수호출 아님 → lite 로 충분, 1/3 가격 |

두 모델명 모두 **config 값**으로 둔다. 2.5 deprecation(2026-06-17 API deprecation)이 임박했으므로 `gemini-3.1-flash` / `gemini-3.1-flash-lite` 로 한 줄 교체 가능해야 한다. 기본값: `GEMINI_MODEL=gemini-2.5-flash`, `GEMINI_VISION_MODEL=gemini-2.5-flash-lite`.

이미지 토큰 비용은 사진 1장 ≈ 0.03센트 수준으로 사실상 무시 가능. 비용을 지배하는 건 매 호출 재전송되는 시스템 프롬프트 + 툴 선언.

## 3. 흐름

```txt
Slack app_mention/DM (files 첨부 포함)
  ↓ slack 어댑터: event.Files 추출 → url_private 를 봇 토큰으로 다운로드
  ↓                → IncomingMessage.Images [{Data, MIME}]
  ↓ Agent.Route:
  ↓   if len(in.Images) > 0:
  ↓       names = VisionExtractor.ExtractItems(ctx, images)   # flash-lite, JSON
  ↓       if len(names) == 0:
  ↓           return "사진에서 물건을 못 찾았어. 뭐가 있는지 말로 알려줄래?"
  ↓       in.Text = "[사진에서 인식한 물건: " + join(names) + "] " + in.Text
  ↓   (이하 기존 에이전트 루프 그대로)
  ↓ 에이전트가 add_items 호출 (장소 resolve + 카테고리 자동지정)
  ↓ 변경안 + 승인 버튼  ← 기존 플로우
```

장소가 텍스트에 없으면(사진만 던짐): 에이전트가 인식한 물건을 보여주고 "어디에 넣은 거야?" 되묻는다 — systemPrompt 지시 + add_items 의 location resolve 실패 시 기존 되묻기 메커니즘으로 자연히 처리된다. 별도 코드 분기 불필요.

## 4. 컴포넌트

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `domain/slack.go` | `IncomingMessage.Images []Image{Data []byte; MIME string}` 추가 | 변경 |
| `internal/slack/client.go` | app_mention/message 이벤트에서 `Files` 추출, `url_private` 봇토큰 GET 다운로드 → Images | 변경 |
| `internal/gemini/client.go` | 비전 메서드 `ExtractItems(ctx, images) ([]string, error)` — 이미지 파트 + JSON 출력. flash-lite Client 인스턴스 | 확장 |
| `internal/agent/agent.go` | `VisionExtractor` 인터페이스 주입. `Route` 앞단에서 이미지 있으면 추출→텍스트 증강 | 변경 |
| `pkg/config/config.go` | `GEMINI_VISION_MODEL` 추가(기본 flash-lite). 미설정 허용(기본값) | 변경 |
| `cmd/server/main.go` | flash-lite gemini Client 추가 생성 → VisionExtractor 로 agent 에 주입 | 변경 |

기존 `add_items` 도구 · `ChangeProposal` · 승인/interaction 흐름 · MapRenderer 는 **무변경**.

## 5. 비전 추출 계약

```go
// VisionExtractor 는 이미지에서 물건 이름 목록을 뽑는다(agent 가 주입받음).
type VisionExtractor interface {
    ExtractItems(ctx context.Context, images []domain.Image) ([]string, error)
}
```

- 출력: 물건 **이름 목록만** (카테고리·수량 분류 안 함 → 단일 책임, lite 로 충분).
- Gemini JSON 출력(`ResponseMIMEType=application/json` + `ResponseSchema` = string 배열) 사용.
- 비전 프롬프트: "이 사진에 보이는 정리/수납 대상 물건들을 한국어 이름의 배열로만 반환해라. 가구·벽·바닥 같은 배경은 제외하고, 옮기거나 수납할 수 있는 물건만. 확실치 않으면 제외." thinking budget=0.
- 여러 장 첨부 시: 모든 이미지를 한 호출의 파트로 넣어 합집합 인식(중복 이름은 추출 후 dedupe).

## 6. Slack 첨부 다운로드

- Slack 이벤트(`MessageEvent`/`AppMentionEvent`)의 `Files []slackevents.File` 에서 `URLPrivate`(또는 `URLPrivateDownload`) 사용.
- 다운로드: `GET url_private` + 헤더 `Authorization: Bearer <SLACK_BOT_TOKEN>` (슬랙 파일은 봇토큰 인증 필요).
- 이미지 MIME(`image/*`)만 취급. 그 외(pdf 등) 무시.
- 다운로드/인증 실패: 로그 + 이미지 없이 **텍스트만으로 진행**(전체 실패 안 시킴).
- 크기 가드: 과대 파일 방지 위해 상한(예: 10MB) 초과 시 스킵.

## 7. 에러 처리 (안전 원칙 유지)

- 쓰기는 여전히 **승인 버튼** 거침(LLM 이 Notion 직접 안 건드림). 사진은 입력 보조일 뿐, 등록은 add_items 변경안 승인으로만.
- 비전 인식 0개 → "사진에서 물건을 못 찾았어. 뭐가 있는지 말로 알려줄래?"
- 비전 호출 실패 → 로그 + 텍스트만으로 진행(있으면) 또는 짧은 안내.
- 다운로드 실패 → 로그 + 텍스트만으로 진행.

## 8. 테스트 전략

- `internal/agent`: fake VisionExtractor 주입 → 이미지 있으면 텍스트가 `[사진에서 인식한 물건: ...]` 로 증강되어 루프에 들어가는지, 0개일 때 되묻기 응답하는지.
- `internal/gemini`: `ExtractItems` 의 요청 빌드(이미지 파트 + JSON 스키마)를 httptest/얇은 단위로. 실제 비전 인식은 라이브 검증.
- `internal/slack`: `Files`→다운로드 부분은 httptest 서버로 url_private GET(봇토큰 헤더) 검증. MIME 필터·크기 가드.
- config: `GEMINI_VISION_MODEL` 기본값/오버라이드.

## 9. 완료 기준 (수동/라이브)

Slack 에서:
- 멘션 `@jarvis 안방 수납장1에 넣었어` + 물건 여러 개 찍힌 사진 → "정리함, 휴지, 물티슈 인식했어" → add_items 변경안 + 버튼 → 승인 → Notion 생성 + 지도 갱신.
- 사진만(장소 없이) → 인식 물건 보여주고 "어디에 넣은 거야?" 되묻기.
- 물건 안 보이는 사진(벽/풍경) → "사진에서 물건을 못 찾았어".
- 이미지 없는 일반 텍스트 → 기존과 동일 동작(회귀 없음).

## 10. 향후

- 사진에서 장소까지 추론(라벨/문맥), 여러 장 서로 다른 장소 일괄.
- 수량 인식(같은 물건 N개).
- 비전 모델 3.1 교체(config 한 줄).
