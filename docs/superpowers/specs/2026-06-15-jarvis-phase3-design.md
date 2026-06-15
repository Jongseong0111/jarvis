# Jarvis — Phase 3: 집 정리 Notion 연동 설계

작성일: 2026-06-15

## 1. 목표

`home.*` intent 를 실제로 처리한다. 자연어 입력을 LLM 이 구조화된 변경안으로 바꾸고,
**서버 코드의 제한된 함수만** Notion 을 수정한다(LLM 은 Notion 을 직접 건드리지 않는다).
물건 추가는 Slack 버튼 **승인 플로우**를 거친 뒤에만 반영한다.

이번 작업의 범위:

1. Notion 3-DB(Locations/Categories/Items) 데이터 모델 + 셋업 가이드
2. `internal/notion` thin REST 클라이언트
3. Phase 2 Gemini 호출을 `internal/gemini` 공유 클라이언트로 추출(enum + JSON)
4. Home 추출기: 텍스트 → 구조화 변경안(현재 장소/카테고리 목록 기반)
5. `home.search`(읽기): 물건 위치 조회
6. `home.add`(쓰기): 변경안 생성 → Slack 버튼 승인 → Notion 반영
7. Slack interactive(버튼) 플러밍

명시적으로 이번 범위가 **아닌** 것: `home.update`/`home.delete`(다음 차수), 한 번에 여러 개 추가(batch),
삭제 기능, 신규 장소/카테고리 자동 생성, 마스터데이터 캐싱. (§11 참조)

## 2. Notion 데이터 모델

형님 설계(3-DB relational)를 채택한다. DB 스키마는 언제든 바꿀 수 있으므로, 코드는 특정 분류 체계에
하드코딩하지 않고 **DB 에 존재하는 장소/카테고리를 읽어서** 동작한다.

### Locations (장소)
| 속성 | 타입 | 비고 |
|---|---|---|
| 이름 | title | 예: "아기 트롤리", "베란다 수납장1" |
| 구역 | select | 예: 거실/안방/베란다 — 표시·검색용 비정규화 |
| 타입 | select | Room / Storage |
| 상위장소 | relation → Locations(self) | 계층(거실 ← 거실장 왼쪽) |
| 설명 | rich_text | 선택 |

### Categories (카테고리)
| 속성 | 타입 | 비고 |
|---|---|---|
| 이름 | title | 예: "아기상비약", "세탁용품" |
| 상위카테고리 | relation → Categories(self) | 계층 |
| 기본장소 | relation → Locations | "어디 넣어야 하지?" 응답용 |

### Items (물건)
| 속성 | 타입 | 비고 |
|---|---|---|
| 이름 | title | 예: "체온계" |
| 카테고리 | relation → Categories | 선택(LLM 이 목록에서 추론) |
| 현재위치 | relation → Locations | 필수 |
| 수량 | number | 선택(없으면 단순 위치 레지스트리) |
| 메모 | rich_text | 선택 |

환경변수 추가:
```env
NOTION_API_KEY=
NOTION_LOCATIONS_DB_ID=
NOTION_CATEGORIES_DB_ID=
NOTION_ITEMS_DB_ID=
```
`config.validate()` 는 Phase 3 기능을 켜기 위해 위 4개를 필수로 검증한다.

### 셋업 가이드(플랜에 단계별 수록)
1. notion.so/my-integrations 에서 internal integration 생성 → `NOTION_API_KEY`(secret) 확보
2. 위 3개 DB 를 Notion 에서 생성(속성/타입/relation 위 표대로)
3. 각 DB 페이지에서 `...` → Connections → 생성한 integration 연결(권한 공유)
4. 각 DB URL 에서 32자 DB ID 추출 → `config/.env` 에 기입
5. 초기 장소/카테고리 데이터 시드(형님 추천 taxonomy) — 코드는 데이터 비의존이라 내용은 자유

## 3. 컴포넌트

| 패키지/파일 | 책임 |
|---|---|
| `internal/notion/client.go` | thin REST 클라이언트(net/http). `QueryDatabase(ctx, dbID, filter)`, `CreatePage(ctx, dbID, props)`. 우리 3 DB 속성에 맞춘 타입드 결과/요청 helper. SDK 미사용 |
| `internal/notion/types.go` | Location/Category/Item DTO + Notion property(JSON) 매핑 |
| `internal/gemini/client.go` | Phase 2 classifier 의 genai 호출을 추출. `GenerateEnum(ctx, prompt, enum)`, `GenerateJSON(ctx, prompt, schema)` |
| `internal/router/gemini_classifier.go` | (리팩터) `internal/gemini.Client.GenerateEnum` 사용. 외부 동작 동일 |
| `internal/workers/home_extractor.go` | Gemini JSON 추출. 입력: 텍스트 + 현재 Location/Category 이름 목록. 출력: `Extracted{Action, Item, Category, Location, Quantity}` |
| `internal/workers/home.go` | (스텁 교체) HomeOrganizerWorker. `Handle`=search/add 분기, `Apply`=승인된 변경안 반영 |

### 3.1 HomeOrganizerWorker 동작

`Handle(ctx, intent, in)`:
- `home.search`: 마스터데이터 로드 → 추출(item 명) → Items 에서 이름 조회
  - 찾으면: 현재위치(+구역) 응답. 여러 개면 목록.
  - 못 찾고 카테고리 추론되면: 그 카테고리의 **기본장소**를 "여기 넣으면 될 것 같아"로 제안.
  - 둘 다 없으면: 못 찾았다고 응답.
- `home.add`: 마스터데이터 로드 → 추출 → location/category 이름을 **page ID 로 resolve**
  - location resolve 실패: 되묻기(등록된 장소 목록 안내).
  - 성공: `ChangeProposal` 생성 → 버튼 달린 Reply 반환.
- `home.update`/`home.delete`: "아직 준비 중이야(다음 단계)" 응답.

`Apply(ctx, proposal)`(ProposalApplier): Items DB 에 page 생성(카테고리/현재위치 relation 연결, 수량/메모 선택) → 확인 응답.

마스터데이터 로드 = Locations + Categories 각 1회 Query(요청당). 개인 저용량이라 캐싱 없이 시작.

## 4. 도메인 확장 (`domain/`)

`Reply` 에 채널 독립적 버튼을 추가한다.

```go
type Reply struct {
	ChannelID string
	Text      string
	Buttons   []Button // 비면 일반 텍스트 응답 (Phase 1/2 와 호환)
}

// Button 은 채널 독립적 액션 버튼이다. Slack 어댑터가 Block Kit 으로 렌더링한다.
type Button struct {
	Text   string // 표시 라벨 ("승인"/"취소")
	Action string // "approve" / "cancel"
	Value  string // 액션에 필요한 직렬화 데이터 (ChangeProposal JSON)
	Style  string // "primary"/"danger"/"" (선택)
}

// ChangeProposal 은 승인 대기 중인 변경안이다. 버튼 value 에 JSON 으로 인코딩된다.
type ChangeProposal struct {
	Action       string // "add"
	ItemName     string
	CategoryID   string // resolve 된 Notion page ID (없으면 빈 값)
	CategoryName string
	LocationID   string // resolve 된 Notion page ID
	LocationName string
	Quantity     *int
}

// ProposalApplier 는 승인된 변경안을 실제 시스템에 반영한다.
type ProposalApplier interface {
	Apply(ctx context.Context, p ChangeProposal) (Reply, error)
}
```

`Reply{Buttons: nil}` 은 기존 동작과 동일하므로 Phase 1/2 코드는 영향 없다.

## 5. 데이터 흐름

### 추가(쓰기)
```txt
"아기 트롤리에 체온계 뒀어"
  → Router → home.add → HomeWorker.Handle
      → 마스터데이터 로드 → 추출 → location/category resolve
      → ChangeProposal 생성
  → Reply{Text: 변경안 요약, Buttons: [승인(value=proposal JSON), 취소]}
  → Slack: 변경안 + 버튼 메시지

[승인] 클릭
  → Slack interactive 이벤트 → value 디코드 → ProposalApplier.Apply
      → notion.CreatePage(Items, ...) → 확인 Reply
[취소] 클릭
  → "취소했어" Reply (Apply 호출 안 함)
```

### 검색(읽기)
```txt
"체온계 어디있지?"
  → Router → home.search → HomeWorker.Handle
      → 추출(item) → notion.QueryDatabase(Items, 이름 filter)
      → 현재위치 relation 따라가 위치명/구역 → 텍스트 Reply (버튼 없음)
```

## 6. 승인 플로우 (stateless)

변경안의 **resolve 된 ID 까지 포함한 ChangeProposal** 을 JSON 직렬화해 버튼 `Value` 에 인코딩한다.
승인 시 그 value 만으로 바로 실행하므로 서버는 pending 상태를 메모리에 들고 있지 않는다(재시작에도 견딤).
Slack action value 한도(2000자)보다 변경안 JSON 이 훨씬 작아 안전하다.

resolve(이름→ID)는 **변경안 생성 시점**에 수행한다. 따라서 사용자는 실제로 생성될 대상을 미리 보고,
resolve 실패 시에는 변경안 대신 되묻기가 나간다(잘못된 대상이 승인되는 일 방지).

## 7. Slack interactive 플러밍 (`internal/slack`)

- `client.go` Run 이벤트 루프에 `socketmode.EventTypeInteractive` 분기 추가 → Ack → interaction 핸들러.
- interaction 핸들러: `slack.InteractionCallback` 에서 클릭된 action/value 추출 → `approve` 면 `ProposalApplier.Apply`, `cancel` 이면 취소 응답 → 결과를 채널에 전송.
- 메시지 전송부: `Reply.Buttons` 가 있으면 Block Kit(section + actions) 로 변환해 전송, 없으면 기존 텍스트 전송.
- 버튼 value 인코딩/디코딩은 순수 함수로 분리(테스트 용이).
- **Slack 앱 설정**: Interactivity 활성화 필요. Socket Mode 이므로 Request URL(공개 엔드포인트)은 불필요.

## 8. 에러 처리 (안전 원칙)

- Notion 호출 실패: 전체 에러 `slog` 로그 + 사용자에겐 짧게("Notion 연동에 문제가 생겼어. 잠시 후 다시 시도해줘").
- location resolve 실패: 변경안 대신 되묻기("'아기방 옷장'을 못 찾았어. 등록된 장소 중에서 알려줄래?").
- LLM 추출 실패: Phase 2 와 동일(로그 + 짧은 안내).
- 삭제/대량 변경은 이번 범위에 없음(추가만, 건당 승인) → 안전 원칙 자동 충족.

## 9. 테스트 전략

- `internal/notion`: `httptest.Server` 로 Notion API 흉내 → Query/Create 요청 직렬화·응답 파싱 검증.
- `internal/gemini`: 프롬프트/스키마 구성 등 순수 부분. 실제 호출은 얇게.
- `home_extractor`: 프롬프트 빌더 + JSON 응답 파싱(`Extracted` 매핑, 누락 필드 처리) 순수 함수.
- `home.go`: fake Notion 클라이언트 + fake 추출기로 — search(찾음/못찾음/기본장소 제안), add(변경안 생성, resolve 실패 되묻기), Apply(CreatePage 호출 검증).
- `domain`: ChangeProposal JSON 라운드트립.
- `internal/slack`: 버튼 value 인코딩/디코딩, `Reply.Buttons`→Block Kit 변환(순수 함수). interaction 핸들러는 fake applier 로.

## 10. 완료 기준 (수동 검증)

Notion DB 3개 셋업 + 시드 후, Slack 에서:

- `@jarvis 아기 트롤리에 체온계 뒀어` → 변경안 + `[승인][취소]` 버튼 표시
- [승인] → Notion Items 에 "체온계"(현재위치=아기 트롤리) 생성됨 + 확인 메시지
- [취소] → 생성 안 됨 + "취소했어"
- `@jarvis 체온계 어디있지?` → "아기 트롤리(거실)" 위치 응답
- `@jarvis 세제 어디 넣어야 하지?` → (체온계 미등록 시) 카테고리 기본장소 제안
- `@jarvis 없는장소xyz에 뭐 넣었어` → 장소 못 찾음 되묻기

## 11. YAGNI / 향후

다음 차수(형님 요청 반영):
- **batch 추가**: 한 메시지로 여러 물건 추가(변경안 N건/일괄 승인)
- **삭제**(`home.delete`): 삭제 후보 생성 → 승인 → 반영(안전 원칙상 항상 승인)
- **수정**(`home.update`): 위치/수량 변경, "건전지 2개 썼어" 수량 감소

이후:
- 신규 장소/카테고리 자동 생성(현재는 기존 것만 resolve)
- 마스터데이터 캐싱(TTL) — 요청당 Query 제거
- MCP 연동(형님 메모의 action JSON 포맷과 정렬)
