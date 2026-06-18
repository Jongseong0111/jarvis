# Jarvis — 지식저장소: ChatGPT 공유링크 요약 (Phase A) 설계

작성일: 2026-06-18

## 1. 배경 / 목표

ChatGPT에서 나눈 대화를 개인 지식저장소(`knowledge-base` 레포)로 끌어오고 싶다. 하지만:

- OpenAI API는 **내 ChatGPT 웹 대화 기록에 접근할 수 없다**(소비자 제품과 개발자 API가 분리됨, 공식 엔드포인트 없음).
- ChatGPT **공유 링크**(`chatgpt.com/share/...`)는 대화 본문이 페이지 HTML에 임베드돼 있어 **서버에서 추출 가능**하다(검증 완료). 단 JS 렌더라 WebFetch로는 안 되고 raw HTTP fetch + 파싱이 필요. 백엔드 API는 Cloudflare 403.

전체 비전(= CLAUDE.md Phase 4):
```
ChatGPT 공유링크 → 슬랙 @jarvis + 링크
  ① 추출(jarvis) → ② 요약 source(jarvis) → ③ 개념 분리(kb skill, Claude Code headless)
  → ④ 슬랙 승인 → ⑤ 커밋
```

이 문서는 **Phase A**만 다룬다: **① 추출 + ② 요약 + sources/ 저장 + 슬랙에 요약 표시.** 가장 불확실한 "추출·요약 품질"을 먼저 검증한다. ③④⑤(Claude Code headless 개념분리 + 승인 + 커밋, 비동기 구조)는 Phase B.

## 2. 범위

**In (Phase A):**
- 슬랙 메시지에서 `chatgpt.com/share/...` 링크를 받아 대화를 추출.
- 대화를 Gemini로 깔끔히 요약(한국어, 정해진 양식) → 슬랙에 **보여주기만**(저장 안 함).
- 사용자가 슬랙에서 **대화로 요약을 수정**("이 부분 빼", "더 짧게", "제목 바꿔")하면 에이전트가 맥락에서 고쳐 다시 보여줌.
- 사용자가 **확정("저장해")하면** 그 시점의 (수정된) 요약을 kb 레포 `sources/conversation/<date>-<slug>.md` 에 저장(커밋 안 함).

**Out (Phase B 이후):**
- 개념별 분리(concept 문서) — kb-ingest(Claude Code headless).
- 승인 버튼 → git 커밋/푸시.
- 비동기 처리(백그라운드 + 나중에 슬랙 알림).
- export 파일(conversations.json) 입력, 멀티도메인 필터.

## 3. 흐름 (대화형: 요약 → 수정 → 저장)

```
슬랙: @jarvis 이 대화 정리해줘 https://chatgpt.com/share/...
  ↓ 에이전트가 공유 링크 + 정리 의도 인식 → summarize_chatgpt_share(url) 도구 호출
  ↓ ① share.FetchConversation(url): raw HTTP GET(브라우저 UA) → 제목 + 대화 텍스트 추출
  ↓ ② gemini.GenerateText(요약 프롬프트, 대화텍스트) → 요약 마크다운
  ↓ 도구가 요약을 문자열로 반환 → 에이전트가 슬랙에 보여줌 (저장 안 함)

슬랙: "고루틴 부분만 남기고 더 짧게"  (수정 요청)
  ↓ 에이전트가 대화 맥락의 요약을 직접 고쳐 다시 보여줌 (도구 호출 없음 — 순수 대화)

슬랙: "저장해"  (확정)
  ↓ 에이전트가 save_kb_source(title, url, content=현재 요약) 도구 호출
  ↓ ③ source.Write(repoPath, title, url, content) → sources/conversation/<date>-<slug>.md
  ↓ 에이전트가 "저장했어: <경로>" 응답
```

각 단계는 한 슬랙 메시지(동기, 30초 내). 요약/수정본은 에이전트 대화 기억에 남아 다음 턴(수정·저장)에서 쓰인다. 별도 승인 버튼 없음 — **"저장해"라는 말이 곧 승인**이고, 요약이 커서 버튼 value(~2KB)에 안 들어가기도 함. 저장 파일은 미커밋이라 가역적(진짜 커밋 승인은 Phase B).

**중요(수정 루프의 핵심):** 수정본은 LLM 대화 맥락에만 존재한다. 그래서 저장 시 **에이전트가 최종 content를 도구 인자로 직접 넘긴다**(서버가 원본을 캐시하면 수정이 반영 안 됨). `save_kb_source` 는 content 를 인자로 받는다.

## 4. 컴포넌트

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `internal/knowledge/share.go` | `FetchConversation(ctx, url) (Conversation, error)` — HTTP fetch + HTML에서 제목·대화 추출 | 신규 |
| `internal/knowledge/source.go` | `WriteSource(repoPath, title, url, content string) (path string, error)` — 소스 노트 파일 작성 | 신규 |
| `internal/knowledge/port.go` | `KnowledgePort` 인터페이스(요약·저장 분리, 테스트용 fake) + `Service` 구현 | 신규 |
| `internal/gemini/generate.go` | `GenerateText(ctx, system, user string) (string, error)` — 도구 없는 일반 텍스트 생성 | 신규 |
| `internal/agent/knowledge_tools.go` | `KnowledgeTools(port) []Tool` — `summarize_chatgpt_share`(읽기) + `save_kb_source`(저장) | 신규 |
| `internal/agent/agent.go` | system 프롬프트에 ChatGPT 링크 처리 지침 추가 | 변경 |
| `pkg/config/config.go` | `KnowledgeRepoPath`(env `KNOWLEDGE_REPO_PATH`, 기본 `~/personal-agent/knowledge-base`, `~` 확장) | 변경 |
| `cmd/server/main.go` | knowledge Service 조립 → 에이전트 도구에 합침 | 변경 |

홈 도구/변경안/승인/맵 렌더러는 **무변경**.

## 5. 추출 계약 (`internal/knowledge/share.go`)

```go
type Conversation struct {
    Title    string   // 페이지 제목에서 "ChatGPT - " 접두 제거
    URL      string
    Messages []string // 대화 메시지(거칠게 추출, 화자 구분 없음)
}

func FetchConversation(ctx context.Context, url string) (Conversation, error)
```

- **검증**: URL이 `chatgpt.com/share/` 또는 `chat.openai.com/share/` 패턴인지 확인. 아니면 에러.
- **fetch**: `net/http` GET, 헤더 `User-Agent: Mozilla/5.0 ...Chrome...`(WebFetch UA로는 셸만 옴), 타임아웃 20s, `--max 1MB` 정도 본문 제한.
- **제목**: `<title>...</title>` → `ChatGPT - ` 접두 제거.
- **대화 추출(휴리스틱)**: HTML에 임베드된 React Router 스트림에서 따옴표로 감싼 자연어 문자열을 뽑는다. 규칙: 길이 ≥ 15, 공백 또는 한글 포함, 순수 식별자(snake_case/영문토큰)·URL·플래그명 제외. 순서 보존. → `Messages`.
  - 이 추출은 **거칠어도 된다**: 요약 LLM이 노이즈를 걸러낸다. 화자 구분/정확한 턴 구조는 불필요(개념 요약이 목적이지 트랜스크립트가 아님).
- **빈 결과**: 메시지가 0개거나 합이 너무 짧으면(<100자) 에러("대화를 추출하지 못했어. 공유가 켜진 링크가 맞아?").

## 6. 요약 계약 (`internal/gemini/generate.go`)

```go
func (c *Client) GenerateText(ctx context.Context, system, user string) (string, error)
```

- 도구 없이 일반 생성(`GenerateWithTools`와 유사하나 Tools 없음, thinking=0, temp=0). 모델은 `c.model`(flash).
- 요약 system 프롬프트(요지): "다음은 ChatGPT 대화를 거칠게 추출한 텍스트다. 한 개발자의 지식저장소에 넣을 **간결한 한국어 요약 노트**로 정리해라. 잡담·인사·중복은 버리고 핵심 개념/결론만. 아래 마크다운 양식을 따라라." + 양식 템플릿(§7).

## 7. 소스 노트 양식 (`internal/knowledge/source.go`)

"적당히"(사용자 위임) — frontmatter + 요약 본문. Gemini가 본문(`##` 섹션)을 채우고, 코드가 frontmatter를 씌운다.

```markdown
---
title: <title>
source: chatgpt-share
url: <url>
captured: <YYYY-MM-DD>
type: conversation
---

<content: Gemini 요약 본문 (## 핵심 / ## 상세 등 자유 구성, 사용자가 수정한 최종본)>
```

- `WriteSource`는 frontmatter를 씌우고 `content`(요약 본문)를 그 아래 붙인다. `url`이 빈 문자열이면 해당 줄 생략.
- 경로: `sources/conversation/<YYYY-MM-DD>-<slug>.md`. slug = title 소문자화 + 비단어→`-` + 60자 컷(기존 kb-ingest 규칙과 동일).
- `sources/conversation/` 디렉터리 없으면 생성.
- **중복**: 같은 경로 존재 시 `-2`, `-3` 접미(덮어쓰기 방지).
- 커밋하지 않음(Phase A). 미커밋 파일이라 사용자가 git/에디터로 쉽게 리뷰·삭제 가능.

## 8. 에이전트 통합

도구 2개, 둘 다 **읽기형(즉시 실행, 문자열 반환)** — 변경안/버튼 안 씀. 저장은 사용자의 명시적 "저장해"가 승인 역할.

- `summarize_chatgpt_share(url string)`: `port.Summarize(ctx, url)` → 추출+요약 → 요약 마크다운 반환(저장 안 함). 에이전트가 슬랙에 보여줌.
- `save_kb_source(title, content string, url string)`: `port.SaveSource(ctx, title, url, content)` → `sources/`에 저장 → "저장했어: <경로>" 반환. `content`는 **에이전트가 대화 맥락의 최종(수정된) 요약을 넘긴다**.

system 프롬프트 추가(요지):
- "사용자가 `chatgpt.com/share/...` 링크를 정리 요청과 함께 보내면 `summarize_chatgpt_share`를 호출하고, **요약을 보여준 뒤 바로 저장하지 마라.**"
- "사용자가 요약 수정을 요청하면 도구 없이 대화로 고쳐 다시 보여줘라."
- "사용자가 '저장/저장해/이대로 저장' 등으로 확정하면 그때 `save_kb_source`를 **현재 요약 전체를 content로** 호출하라."

`KnowledgePort` 인터페이스로 주입(테스트 fake):
```go
type KnowledgePort interface {
    Summarize(ctx context.Context, url string) (title string, summary string, err error)
    SaveSource(ctx context.Context, title, url, content string) (path string, err error)
}
```
`Service`(share + gemini + source 조합)가 구현. `title`은 `Summarize`가 추출해 반환하고, 저장 시 에이전트가 그 제목을 `save_kb_source`에 넘긴다.

## 9. 에러 처리

- 비공유/잘못된 링크, fetch 실패, 추출 빈약 → 도구가 에러 반환 → 에이전트가 짧게 안내("이 링크에서 대화를 못 가져왔어. 공유가 켜진 ChatGPT 링크가 맞아?").
- Gemini 실패 → 에러 전파 → "요약 중 문제가 생겼어."
- 파일 쓰기 실패 → 에러 전파.
- 안전: Phase A는 **외부/파괴적 변경 없음**(로컬 미커밋 파일 1개). 커밋/푸시는 Phase B에서 승인 거침.

## 10. 테스트 전략

- `internal/knowledge/share.go`: 저장된 실제 share HTML **픽스처**(`testdata/share-go.html`)로 `FetchConversation`의 파싱부 검증 — 제목 추출, 핵심 메시지("고루틴"/"채널" 등) 포함, 플래그/식별자 노이즈 제외. 실제 네트워크 fetch는 라이브 검증.
- `internal/knowledge/source.go`: 임시 디렉터리에 `WriteSource` → 경로·frontmatter·중복 접미 검증.
- `internal/gemini/generate.go`: 네트워크라 라이브 검증(기존 컨벤션).
- `internal/agent`: fake `KnowledgePort` → `summarize_chatgpt_share`가 요약 반환·저장 안 함, `save_kb_source`가 넘긴 content로 SaveSource 호출하고 경로 반환하는지.
- 전 패키지 `go test/vet/build` green.

## 11. 완료 기준 (수동/라이브)

슬랙에서:
- `@jarvis 이 대화 정리해줘 <Go 공유링크>` → Go 동시성 요약이 슬랙에 옴(아직 저장 안 됨).
- 이어서 "더 짧게" / "고루틴 부분만" → 수정된 요약이 다시 옴.
- "저장해" → `knowledge-base/sources/conversation/2026-06-18-고랭-장점-설명.md` 생성(수정 반영된 내용).
- 비공유/깨진 링크 → "대화를 못 가져왔어" 안내.
- 일반 메시지(링크 없음) → 기존 동작 회귀 없음.

## 12. Phase B 예고 (이번 범위 아님)

- Claude Code headless로 `kb-ingest` 실행 → 소스에서 **개념별 concept 문서** 분리(사용자 "개념별로 나누기" 요구).
- 슬랙 승인 버튼 → git diff 리뷰 → 커밋(+선택 push). 안전원칙: 승인 전 커밋 금지.
- 느린 작업이라 **비동기**(고루틴 + 완료 시 슬랙 알림) 구조 도입.
- kb-ingest 스킬의 옛 경로(`acloset-agent/base/...`) 수정 필요.
