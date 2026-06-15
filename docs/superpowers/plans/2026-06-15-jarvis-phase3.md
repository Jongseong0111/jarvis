# Phase 3: 집 정리 Notion 연동 Implementation Plan

> **For agentic workers:** 이 플랜은 `docs/superpowers/specs/2026-06-15-jarvis-phase3-design.md` 를 구현한다. 순서대로 task 단위로 진행하고, 각 task 끝에 `go test ./... -race` 통과 후 커밋한다.

**Goal:** `home.add`(버튼 승인 후 Notion 반영)와 `home.search`(위치 조회)를 실제 Notion 3-DB 연동으로 구현한다.

**Architecture:** Slack Handler → Router → HomeWorker. HomeWorker 는 Gemini 추출기로 자연어를 구조화하고, thin Notion 클라이언트로 조회/생성한다. 추가는 stateless 버튼(변경안을 value 에 인코딩) 승인 후에만 반영. Notion 의 rollup+뷰가 대시보드 표현을 담당(코드 외).

**Tech Stack:** Go 1.25, `google.golang.org/genai`(Gemini), Notion REST API(net/http), slack-go(Block Kit + Socket Mode interactive).

---

## 파일 구조

| 파일 | 책임 | 신규/수정 |
|---|---|---|
| `domain/router.go` | Reply.Buttons, Button, ChangeProposal, ProposalApplier 추가 | 수정 |
| `internal/gemini/client.go` | 공유 Gemini 클라이언트(GenerateEnum/GenerateJSON) | 신규 |
| `internal/router/gemini_classifier.go` | gemini.Client 사용하도록 리팩터 | 수정 |
| `internal/notion/types.go` | Location/Category/Item DTO + property 매핑 | 신규 |
| `internal/notion/client.go` | QueryDatabase/CreatePage(net/http) | 신규 |
| `internal/workers/home_extractor.go` | Gemini JSON 추출기 + 인터페이스 | 신규 |
| `internal/workers/home.go` | HomeWorker(Handle/Apply) — 스텁 교체 | 수정 |
| `internal/slack/blocks.go` | Reply.Buttons → Block Kit, value 인코딩/디코딩 | 신규 |
| `internal/slack/handler.go` | 변경 없음(라우터 위임 유지) | - |
| `internal/slack/client.go` | EventTypeInteractive 처리 추가 | 수정 |
| `internal/slack/interaction.go` | 버튼 클릭 → applier.Apply/취소 | 신규 |
| `pkg/config/config.go` | NOTION_* 4개 추가 | 수정 |
| `cmd/server/main.go` | notion/gemini/home 조립 | 수정 |

---

## Task 1: 도메인 확장 (Reply.Buttons / ChangeProposal)

**Files:** Modify `domain/router.go`, Test `domain/router_test.go`

- [ ] Reply 에 `Buttons []Button`, 신규 `Button{Text,Action,Value,Style}`, `ChangeProposal{Action,ItemName,CategoryID,CategoryName,LocationID,LocationName,Quantity *int}`, `ProposalApplier` 인터페이스 추가 (spec §4 그대로).
- [ ] `ChangeProposal` JSON 라운드트립 테스트(`encoding/json` Marshal→Unmarshal 동일성, Quantity nil/값 둘 다).
- [ ] `go test ./domain/ -race` 통과 → 커밋.

## Task 2: 공유 Gemini 클라이언트

**Files:** Create `internal/gemini/client.go`, Modify `internal/router/gemini_classifier.go`(+test)

- [ ] `gemini.Client{apiKey, model}` + `New(apiKey, model)`. 메서드:
  - `GenerateEnum(ctx, prompt string, enum []string) (string, error)` — Phase 2 의 enum 호출 이전.
  - `GenerateJSON(ctx, prompt string, schema *genai.Schema) (string, error)` — `ResponseMIMEType:"application/json"` + schema, `resp.Text()` 반환.
  - 공통: genai.NewClient(BackendGeminiAPI), 15s 타임아웃, Temperature 0.
- [ ] classifier 리팩터: `GeminiClassifier` 가 `*gemini.Client` 보유, `Classify` 는 `client.GenerateEnum(buildClassifyPrompt(text), enumValues())` 호출 후 `validateIntent`. 기존 순수함수(buildClassifyPrompt/enumValues/validateIntent)·테스트 유지.
- [ ] `NewGeminiClassifier(apiKey, model)` 시그니처 유지(내부에서 gemini.New 생성) → main.go 영향 없음.
- [ ] `go test ./internal/router/ ./internal/gemini/ -race` 통과 → 커밋.

## Task 3: Notion 클라이언트 + 타입

**Files:** Create `internal/notion/types.go`, `internal/notion/client.go`, `internal/notion/client_test.go`

- [ ] `types.go`: `Location{ID,Name,Zone}`, `Category{ID,Name,DefaultLocationID}`, `Item{ID,Name,CategoryName,LocationName,Zone}`. Notion property JSON 파싱 helper(title/select/relation 추출), CreatePage 용 property 빌더.
- [ ] `client.go`: `Client{apiKey, http, baseURL}` + `New(apiKey)`. 
  - `QueryDatabase(ctx, dbID string, filter any) ([]Page, error)` — POST `/v1/databases/{id}/query`, `Notion-Version` 헤더, Bearer 인증.
  - `CreatePage(ctx, dbID string, props map[string]any) (string, error)` — POST `/v1/pages`, 생성된 page ID 반환.
  - `baseURL` 은 테스트에서 교체 가능(overridable).
- [ ] `client_test.go`: `httptest.Server` 로 — Query 요청 헤더/바디 검증 + 가짜 응답 파싱, Create 요청 바디 검증 + ID 반환.
- [ ] `go test ./internal/notion/ -race` 통과 → 커밋.

## Task 4: config NOTION_* 추가

**Files:** Modify `pkg/config/config.go`(+test), `config/.env.example`

- [ ] Config 에 `NotionAPIKey, NotionLocationsDBID, NotionCategoriesDBID, NotionItemsDBID` 추가, New 에서 로드, validate 에서 4개 필수.
- [ ] config_test 정상 케이스에 NOTION_* 채우고, 누락 케이스 추가.
- [ ] `.env.example` 에 4개 키 추가.
- [ ] `go test ./pkg/config/ -race` 통과 → 커밋.

## Task 5: Home 추출기

**Files:** Create `internal/workers/home_extractor.go`, `internal/workers/home_extractor_test.go`

- [ ] `Extracted{Action string; Item string; Category string; Location string; Quantity *int}`.
- [ ] `Extractor` 인터페이스 `Extract(ctx, text string, locations, categories []string) (Extracted, error)`.
- [ ] `GeminiExtractor{client *gemini.Client}` 구현: 프롬프트(텍스트 + 장소/카테고리 목록 + action 설명) + `GenerateJSON(schema)` → `Extracted` 언마샬.
- [ ] 순수함수 `buildExtractPrompt`, `extractSchema()` 분리. 테스트: 프롬프트에 입력 포함, JSON→Extracted 파싱(quantity 유무).
- [ ] `go test ./internal/workers/ -race` 통과 → 커밋.

## Task 6: HomeWorker (search/add/Apply)

**Files:** Modify `internal/workers/home.go`(스텁 교체), Test `internal/workers/home_test.go`

- [ ] `HomeWorker{notion NotionPort; extractor Extractor; cfg DBIDs}`. `NotionPort` 인터페이스(QueryDatabase/CreatePage)로 fake 주입 가능.
- [ ] `Handle`:
  - 마스터데이터 로드(Locations/Categories Query) → 이름 목록.
  - `home.search`: Extract → Items 이름 필터 Query → 있으면 위치(+구역), 없고 카테고리 추론되면 기본장소 제안, 아니면 못 찾음.
  - `home.add`: Extract → location 이름→ID resolve(실패 시 되묻기 Reply), category 이름→ID resolve(선택) → `ChangeProposal` → `proposalReply`(버튼 2개, value=JSON).
  - update/delete: "준비 중".
- [ ] `Apply(ctx, p ChangeProposal)`: Items props 빌드(이름 title, 현재위치/카테고리 relation, 수량) → `CreatePage` → 확인 Reply.
- [ ] 순수함수 `proposalReply(p)`(버튼 구성), `resolveByName(name, list)`.
- [ ] 테스트: fake NotionPort+fake Extractor 로 search(찾음/기본장소/못찾음), add(변경안/resolve 실패), Apply(CreatePage 호출·props 검증).
- [ ] `go test ./internal/workers/ -race` 통과 → 커밋.

## Task 7: Slack 버튼 렌더링 + interactive

**Files:** Create `internal/slack/blocks.go`, `internal/slack/interaction.go`, Modify `internal/slack/client.go`, Test `internal/slack/blocks_test.go`

- [ ] `blocks.go`: `buildBlocks(reply domain.Reply) []slackgo.Block`(Text section + Buttons→ActionBlock), `encodeProposal/decodeProposal`(JSON). Send 에서 Buttons 있으면 blocks 사용.
- [ ] Client.Send: `Reply.Buttons` 있으면 `MsgOptionBlocks(...)` 로 전송.
- [ ] `interaction.go`: `InteractionHandler{applier domain.ProposalApplier, sender domain.MessageSender}`. `Handle(ctx, callback)`: action=approve → decode→Apply→send 결과; cancel → "취소했어" send.
- [ ] `client.go` Run 루프: `socketmode.EventTypeInteractive` 분기 → Ack → `InteractionHandler.Handle`.
- [ ] 테스트: encode/decode 라운드트립, buildBlocks 구조, interaction 핸들러(fake applier/sender).
- [ ] `go test ./internal/slack/ -race` 통과 → 커밋.

## Task 8: 조립 + 전체 검증

**Files:** Modify `cmd/server/main.go`

- [ ] gemini.Client 1개 생성 → classifier + extractor 공유. notion.New(cfg). HomeWorker 생성, router workers["home"]=homeWorker. slack.NewClient 에 InteractionHandler(applier=homeWorker) 연결.
- [ ] `go build ./... && go vet ./... && go test ./... -race` 전부 통과 → 커밋.
- [ ] (토큰 확보 후) §10 라이브 검증 — 임시 통합 테스트로 Notion 연동 확인 후 삭제.

---

## Self-Review 메모
- spec §1-11 커버: 도메인(T1)/gemini(T2)/notion(T3)/config(T4)/추출(T5)/worker(T6)/slack버튼(T7)/조립(T8). 대시보드·rollup 은 코드 외(셋업 가이드 §2).
- 토큰 미보유 → T1~T8 은 fake/httptest 로 완결, 라이브 검증만 보류.
