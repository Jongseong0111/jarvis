# Jarvis — Agent 재설계 (분류기 → 도구 가진 LLM 에이전트) 설계

작성일: 2026-06-16

## 1. 배경 / 문제

Phase 2~3 는 "엄격한 enum 분류기 → 고정 Worker 디스패치" 구조였다. 결과적으로:

- 정해진 버킷(home.add/search 등) 밖이면 전부 `system.unknown` 으로 거부 → **기본 대화가 안 됨**("안녕" → "무슨 작업인지 모르겠어").
- home 안에서도 **물건 추가/검색 2개만** 가능 → 장소·구역·카테고리 조회/등록/수정 불가.
- LLM 을 "분류 1회"에만 써서 편협함.

근본 원인은 구조다. **LLM 을 도구(tool)를 가진 에이전트로 쓰면** 자연 대화와 유연한 집정리 작업이 한 번에 풀린다.

## 2. 목표 구조

```txt
Slack 메시지
  → Agent (Gemini function calling 루프)
      ├ 잡담/인사 → 자연스럽게 텍스트 응답
      └ 집정리 작업 → 적절한 도구(tool) 호출
          · 읽기 도구: 즉시 실행 → 결과를 모델에 돌려주고 자연어로 답
          · 쓰기 도구: 실행 안 함 → 변경안(ChangeProposal) + 승인 버튼 반환
  → Slack 응답 (텍스트 또는 버튼)

[승인] 클릭 → ProposalApplier 가 변경안 실행(Notion 쓰기)
```

**은퇴**: `internal/router`(classifier+Router), `home_extractor`. **유지·재사용**: `internal/notion`, `internal/gemini`(확장), Slack 핸들러/클라이언트/버튼/interaction, `domain.ProposalApplier` + 승인 흐름.

이번 범위: **자연 대화 + 조회 + 등록**. 범위 밖(다음 차수): 수정/삭제, 멀티턴 대화 기억, 지식저장소·todo 도구.

## 3. 에이전트 루프 (`internal/agent`)

```txt
contents = [user 메시지]
loop (최대 N=5 회):
  resp = gemini.GenerateWithTools(contents, tools, systemPrompt)
  fc = resp 의 첫 FunctionCall
  if fc == nil:
      return 텍스트 Reply(resp.Text())          # 자연 대화/최종 답변
  tool = tools[fc.Name]
  if tool.Write:
      proposal = tool.Propose(fc.Args)           # 실행 안 함 — 이름→ID resolve 포함
      return 버튼 Reply(proposal)                 # 승인 흐름으로
  else:
      result = tool.Run(fc.Args)                  # 읽기 즉시 실행
      contents += [모델의 functionCall content, functionResponse(result) content]
      continue                                    # 모델이 결과로 답을 만들도록 루프
```

- 루프 상한 N(예: 5)으로 무한루프 방지. 초과 시 짧은 안내.
- **메시지 간 상태 없음(stateless)**: 한 Slack 메시지 = 한 루프. 멀티턴 기억은 다음 차수.
- systemPrompt: "친근한 한국어 집정리 비서. 잡담은 자연스럽게. 집정리 작업은 도구 사용. 장소/구역/카테고리는 가능하면 기존 것에서 고르고, 애매하면 되묻기. 응답은 짧고 명확하게."

### 3.1 도구 추상화

```go
type Tool struct {
	Decl  *genai.FunctionDeclaration              // 이름/설명/파라미터 스키마
	Write bool                                     // 쓰기면 승인 필요
	Run   func(ctx, args map[string]any) (string, error)            // 읽기 전용
	Propose func(ctx, args map[string]any) (domain.ChangeProposal, error) // 쓰기 전용
}
```

읽기 도구는 `Run`, 쓰기 도구는 `Propose` 만 채운다.

## 4. 도구 목록 (v1: 조회 + 등록)

읽기(즉시 실행):
| 도구 | 인자 | 동작 |
|---|---|---|
| `list_zones` | — | 구역 목록(Locations 의 구역 distinct) |
| `list_locations` | zone? | 장소 목록(구역 필터 선택) |
| `list_items` | zone?, location? | 물건 목록(구역/장소 필터 선택) |
| `search_item` | name | 이름으로 물건+현재위치 조회 |
| `list_categories` | — | 카테고리 목록(+기본장소) |

쓰기(변경안 → 승인):
| 도구 | 인자 | 동작 |
|---|---|---|
| `add_location` | name, zone | 새 장소 등록안 |
| `add_item` | name, location, category?, quantity? | 새 물건 등록안 |

쓰기 도구 `Propose` 는 이름→ID resolve 를 수행한다(없는 장소/카테고리면 error → 에이전트가 되묻기). 단 `add_location` 의 zone 은 신규 구역도 허용(select 값은 자유).

## 5. 변경안 일반화 (`domain`)

기존 add-item 전용 ChangeProposal 을 op 기반으로 일반화한다.

```go
type ChangeProposal struct {
	Op      string            `json:"op"`      // "add_item" | "add_location"
	Summary string            `json:"summary"` // 버튼 메시지 본문(사람용 요약)
	Fields  map[string]string `json:"fields"`  // 실행에 필요한 resolved 값
}
```

- `add_item` Fields: `name, location_id, location_name, zone, category_id?, category_name?, quantity?`
- `add_location` Fields: `name, zone`
- `Encode()/DecodeProposal()` 유지(버튼 value 인코딩). `Reply.Buttons` 구조 유지.

## 6. 승인 적용 (`ProposalApplier`)

`Apply(ctx, p)` 가 `p.Op` 로 분기:
- `add_item` → `notion.CreatePage(Items, ItemProperties(...))`
- `add_location` → `notion.CreatePage(Locations, LocationProperties(name, zone))`

신규 `notion.LocationProperties(name, zone)` 추가(이름 title + 구역 select). Notion 어댑터에 `CreateLocation` 추가.

## 7. 컴포넌트

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `internal/gemini/client.go` | `GenerateWithTools(ctx, contents, tools, system)` 추가 | 확장 |
| `internal/agent/agent.go` | 에이전트 루프(domain.MessageRouter 구현) | 신규 |
| `internal/agent/tools.go` | Tool 추상화 + 등록 | 신규 |
| `internal/agent/home_tools.go` | home 도구 구현(Notion 어댑터 사용) | 신규 |
| `internal/agent/applier.go` | ProposalApplier(op 분기) | 신규 |
| `internal/notion/*` | `LocationProperties`, 어댑터 `CreateLocation`, 필터 list | 확장 |
| `domain/router.go` | ChangeProposal 일반화. Classifier/Worker/Intent 제거(또는 미사용) | 변경 |
| `internal/slack/*` | 핸들러가 Agent(MessageRouter) 호출. 버튼/interaction 유지 | 유지 |
| `internal/router/*`, `internal/workers/home_extractor.go` | 은퇴(삭제) | 제거 |
| `cmd/server/main.go` | agent 조립 | 변경 |

> `domain.MessageRouter` 인터페이스(`Route(ctx,in)->Reply`)는 유지하고, Agent 가 이를 구현한다 → Slack 핸들러는 그대로.

## 8. 에러 처리 (안전 원칙 유지)

- 쓰기는 **항상 승인 버튼** 거침(LLM 이 Notion 직접 안 건드림 — 도구가 제한된 함수).
- 도구 실행 실패: 로그 + 짧은 안내. resolve 실패: 에이전트가 되묻기.
- Gemini 호출 실패: 로그 + "처리 중 문제가 생겼어".
- 루프 상한 초과: "조금 복잡한 요청이야. 더 구체적으로 말해줄래?"

## 9. 테스트 전략

- `internal/agent`: fake gemini(고정 FunctionCall/텍스트 반환) + fake 도구로 루프 검증 — 잡담→텍스트, 읽기도구→결과반영→답, 쓰기도구→버튼반환, 루프상한.
- home 도구: fake NotionPort 로 list/search/propose(resolve 실패 포함).
- applier: op 분기별 Notion 호출 검증.
- notion: `LocationProperties`, `CreateLocation`(httptest).
- 실제 Gemini function calling 호출부는 얇게 + 라이브 검증(토큰 보유 중).

## 10. 완료 기준 (수동/라이브)

Slack 에서:
- `안녕` → 자연스러운 인사(로봇 거부 ❌)
- `구역 뭐 있어?` / `장소 목록` → 목록 응답
- `거실에 뭐 있어?` → 거실 물건 목록
- `체온계 어디있어?` → 위치 응답
- `팬트리를 거실복도에 추가해줘` → 장소 등록안 + 버튼 → 승인 → Notion 생성
- `아기 트롤리에 손톱깎이 뒀어` → 물건 등록안 + 버튼 → 승인 → 생성

## 11. 향후

- 수정/삭제 도구(update_item/location, delete_*)
- 멀티턴 대화 기억(세션 컨텍스트)
- 지식저장소·todo 도구(같은 에이전트에 도구만 추가)
- 마스터데이터 캐싱
