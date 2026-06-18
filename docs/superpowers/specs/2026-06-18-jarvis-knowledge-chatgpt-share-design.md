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
- 대화를 Gemini로 깔끔히 요약(한국어, 정해진 양식).
- 요약을 kb 레포 `sources/conversation/<date>-<slug>.md` 에 **직접 저장**(커밋 안 함).
- 슬랙에 요약 + 저장 경로 응답.

**Out (Phase B 이후):**
- 개념별 분리(concept 문서) — kb-ingest(Claude Code headless).
- 승인 버튼 → git 커밋/푸시.
- 비동기 처리(백그라운드 + 나중에 슬랙 알림).
- export 파일(conversations.json) 입력, 멀티도메인 필터.

## 3. 흐름 (동기, 빠름)

```
슬랙: @jarvis 이 대화 정리해줘 https://chatgpt.com/share/...
  ↓ 에이전트가 공유 링크 + 정리 의도 인식 → ingest_chatgpt_share(url) 도구 호출
  ↓ ① share.FetchConversation(url): raw HTTP GET(브라우저 UA) → 제목 + 대화 텍스트 추출
  ↓ ② gemini.GenerateText(요약 프롬프트, 대화텍스트) → 요약 마크다운
  ↓ ③ source.Write(repoPath, title, url, 요약) → sources/conversation/<date>-<slug>.md
  ↓ 도구가 요약 + 저장 경로를 문자열로 반환
  ↓ 에이전트가 요약을 슬랙에 응답
```

curl(~1s) + Gemini 요약(~2-5s) → 30초 타임아웃 내 동기 처리 가능. 승인 버튼 불필요(저장 파일은 미커밋이라 가역적).

## 4. 컴포넌트

| 패키지/파일 | 책임 | 상태 |
|---|---|---|
| `internal/knowledge/share.go` | `FetchConversation(ctx, url) (Conversation, error)` — HTTP fetch + HTML에서 제목·대화 추출 | 신규 |
| `internal/knowledge/source.go` | `WriteSource(repoPath string, c Conversation, summary string) (path string, error)` — 소스 노트 파일 작성 | 신규 |
| `internal/knowledge/port.go` | `KnowledgePort` 인터페이스(추출+요약+저장 묶음, 테스트용 fake) + `Service` 구현 | 신규 |
| `internal/gemini/generate.go` | `GenerateText(ctx, system, user string) (string, error)` — 도구 없는 일반 텍스트 생성 | 신규 |
| `internal/agent/knowledge_tools.go` | `KnowledgeTools(port) []Tool` — `ingest_chatgpt_share` 읽기형 도구 | 신규 |
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
title: <Conversation.Title>
source: chatgpt-share
url: <Conversation.URL>
captured: <YYYY-MM-DD>
type: conversation
---

<Gemini 요약 본문 (## 핵심 / ## 상세 등 자유 구성)>
```

- 경로: `sources/conversation/<YYYY-MM-DD>-<slug>.md`. slug = title 소문자화 + 비단어→`-` + 60자 컷(기존 kb-ingest 규칙과 동일).
- `sources/conversation/` 디렉터리 없으면 생성.
- **중복**: 같은 경로 존재 시 `-2`, `-3` 접미(덮어쓰기 방지).
- 커밋하지 않음(Phase A). 미커밋 파일이라 사용자가 git/에디터로 쉽게 리뷰·삭제 가능.

## 8. 에이전트 통합

- 도구 `ingest_chatgpt_share(url string)` — **읽기형**(즉시 실행, 결과 문자열 반환). 변경안/버튼 없음.
- `Run`: `port.Ingest(ctx, url)` 호출 → 추출+요약+저장 → "요약했어:\n\n<요약>\n\n📁 sources/conversation/...에 저장" 반환.
- system 프롬프트 추가: "사용자가 `chatgpt.com/share/...` 링크를 정리/저장 요청과 함께 보내면 `ingest_chatgpt_share` 도구를 그 URL로 호출한다."
- `KnowledgePort` 인터페이스로 주입(테스트 fake):
  ```go
  type KnowledgePort interface {
      Ingest(ctx context.Context, url string) (summary string, path string, err error)
  }
  ```
  `Service`(share + gemini + source 조합)가 구현.

## 9. 에러 처리

- 비공유/잘못된 링크, fetch 실패, 추출 빈약 → 도구가 에러 반환 → 에이전트가 짧게 안내("이 링크에서 대화를 못 가져왔어. 공유가 켜진 ChatGPT 링크가 맞아?").
- Gemini 실패 → 에러 전파 → "요약 중 문제가 생겼어."
- 파일 쓰기 실패 → 에러 전파.
- 안전: Phase A는 **외부/파괴적 변경 없음**(로컬 미커밋 파일 1개). 커밋/푸시는 Phase B에서 승인 거침.

## 10. 테스트 전략

- `internal/knowledge/share.go`: 저장된 실제 share HTML **픽스처**(`testdata/share-go.html`)로 `FetchConversation`의 파싱부 검증 — 제목 추출, 핵심 메시지("고루틴"/"채널" 등) 포함, 플래그/식별자 노이즈 제외. 실제 네트워크 fetch는 라이브 검증.
- `internal/knowledge/source.go`: 임시 디렉터리에 `WriteSource` → 경로·frontmatter·중복 접미 검증.
- `internal/gemini/generate.go`: 네트워크라 라이브 검증(기존 컨벤션).
- `internal/agent`: fake `KnowledgePort` → 도구가 요약+경로 문자열 반환하는지.
- 전 패키지 `go test/vet/build` green.

## 11. 완료 기준 (수동/라이브)

슬랙에서:
- `@jarvis 이 대화 정리해줘 <Go 공유링크>` → Go 동시성 요약이 슬랙에 오고, `knowledge-base/sources/conversation/2026-06-18-고랭-장점-설명.md` 생성.
- 비공유/깨진 링크 → "대화를 못 가져왔어" 안내.
- 일반 메시지(링크 없음) → 기존 동작 회귀 없음.

## 12. Phase B 예고 (이번 범위 아님)

- Claude Code headless로 `kb-ingest` 실행 → 소스에서 **개념별 concept 문서** 분리(사용자 "개념별로 나누기" 요구).
- 슬랙 승인 버튼 → git diff 리뷰 → 커밋(+선택 push). 안전원칙: 승인 전 커밋 금지.
- 느린 작업이라 **비동기**(고루틴 + 완료 시 슬랙 알림) 구조 도입.
- kb-ingest 스킬의 옛 경로(`acloset-agent/base/...`) 수정 필요.
