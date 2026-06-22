# 대화형 공부 주제 재생성 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Slack 대화로 "다른 공부 주제 줘", "운영체제 주제 줘" 같은 요청 시 공부 주제를 재생성하는 `suggest_study_topics` 도구를 jarvis 에이전트에 추가한다.

**Architecture:** `devdigest` 패키지에 주제 전용 생성 메서드 `GenerateTopics` 를 추가(기존 `domains`/`systemPrompt`/`parseResponse` 재사용)하고, `agent` 패키지에 이를 호출하는 읽기 도구 `suggest_study_topics` 를 추가한다. main.go 에서 배선하고 시스템 프롬프트에 사용 힌트를 1줄 더한다.

**Tech Stack:** Go 1.25, `google.golang.org/genai`, 기존 `internal/gemini` 클라이언트

## Global Constraints

- 모듈: `github.com/Jongseong0111/jarvis`, Go 1.25
- 로깅: `pkg/log` slog 래퍼
- 에러: `fmt.Errorf("...: %w", err)` — 커스텀 에러 타입 없음
- 테스트: `t.Parallel()`, fake 인터페이스, 정적 데이터
- 커밋: 한국어 메시지, `feat/fix/test/docs:` 접두
- 도메인 목록(11개, devdigest.domains): 언어 / 웹·백엔드 / 데이터베이스 / 인프라 / 데이터 / 운영체제 / 네트워크 / 자료구조·알고리즘 / 개발도구 / AI / 기타
- 무상태: 아침 digest 와 연동/저장 없음

---

## 파일 구조

| 파일 | 역할 |
|---|---|
| `internal/devdigest/digest.go` | `TopicResult` + `GenerateTopics` + `buildTopicPrompt` 추가 |
| `internal/devdigest/digest_test.go` | `buildTopicPrompt` 분기 테스트 |
| `internal/agent/study_tools.go` | `StudyTopicGenerator` 포트 + `StudyTools` + `suggest_study_topics` 도구 |
| `internal/agent/study_tools_test.go` | fake generator 로 도구 동작 검증 |
| `internal/agent/agent.go` | `DefaultSystemPrompt` 에 힌트 1줄 추가 |
| `cmd/server/main.go` | 도구 배선 |

---

### Task 1: devdigest.GenerateTopics + buildTopicPrompt

**Files:**
- Modify: `internal/devdigest/digest.go`
- Test: `internal/devdigest/digest_test.go`

**Interfaces:**
- Consumes: 기존 `systemPrompt`, `domains`, `parseResponse`, `gemini.Client.GenerateText`
- Produces:
  - `devdigest.TopicResult{Domain string, Topics []string}`
  - `func (g *GeminiGenerator) GenerateTopics(ctx context.Context, requestedDomain string) (TopicResult, error)`
  - `func buildTopicPrompt(requestedDomain string) string` (비공개)

- [ ] **Step 1: buildTopicPrompt 테스트 작성**

`internal/devdigest/digest_test.go` 끝에 추가:

```go
func TestBuildTopicPrompt_specificDomain(t *testing.T) {
	t.Parallel()
	p := buildTopicPrompt("운영체제")
	if !strings.Contains(p, "운영체제") {
		t.Fatalf("지정 도메인 미포함: %q", p)
	}
	// 계층형 형식 안내가 있어야 한다.
	if !strings.Contains(p, "→") {
		t.Fatalf("계층형 형식 안내 없음: %q", p)
	}
	// 출력 JSON 스키마 안내(domain, topics).
	if !strings.Contains(p, "topics") || !strings.Contains(p, "domain") {
		t.Fatalf("JSON 스키마 안내 없음: %q", p)
	}
}

func TestBuildTopicPrompt_randomDomain(t *testing.T) {
	t.Parallel()
	p := buildTopicPrompt("")
	// 미지정 시 11개 도메인 목록이 제시되어야 한다.
	if !strings.Contains(p, "운영체제") || !strings.Contains(p, "데이터베이스") || !strings.Contains(p, "자료구조·알고리즘") {
		t.Fatalf("도메인 목록 미포함: %q", p)
	}
}
```

- [ ] **Step 2: 테스트 실행해서 실패 확인**

Run: `go test ./internal/devdigest/ -run TestBuildTopicPrompt -v 2>&1 | head -10`
Expected: FAIL (`buildTopicPrompt` 미정의 — 컴파일 에러)

- [ ] **Step 3: digest.go 에 TopicResult + GenerateTopics + buildTopicPrompt 구현**

`internal/devdigest/digest.go` 에서, `DigestResult` 타입 정의 아래에 추가:

```go
// TopicResult 는 공부 주제 생성 결과다(뉴스 없이 주제만).
type TopicResult struct {
	Domain string
	Topics []string
}
```

그리고 `Generate` 메서드 아래(또는 파일 하단 buildPrompt 위)에 추가:

```go
// GenerateTopics 는 공부 주제만 생성한다(대화형 재요청용).
// requestedDomain 이 비면 모델이 11개 도메인 중 하나를 선택하고,
// 지정되면 그 도메인(또는 더 구체적인 세부 주제 힌트)으로 계층형 주제를 만든다.
func (g *GeminiGenerator) GenerateTopics(ctx context.Context, requestedDomain string) (TopicResult, error) {
	raw, err := g.client.GenerateText(ctx, systemPrompt, buildTopicPrompt(requestedDomain))
	if err != nil {
		return TopicResult{}, fmt.Errorf("gemini 공부주제 생성 실패: %w", err)
	}
	result, err := parseResponse(raw)
	if err != nil {
		return TopicResult{}, err
	}
	return TopicResult{Domain: result.Domain, Topics: result.Topics}, nil
}

// buildTopicPrompt 는 공부 주제 전용 프롬프트를 만든다.
func buildTopicPrompt(requestedDomain string) string {
	var sb strings.Builder
	sb.WriteString("개발 공부 주제를 생성하라.\n")
	if requestedDomain != "" {
		sb.WriteString("- 도메인/주제: \"" + requestedDomain + "\" 에 대해 생성하라(더 구체적인 세부 주제여도 좋다).\n")
		sb.WriteString("- domain 필드에는 위 주제의 큰 분류명을 넣어라.\n")
	} else {
		sb.WriteString("- 아래 도메인 중 하나를 선택: " + strings.Join(domains, " / ") + "\n")
		sb.WriteString("- 인프라 선택 시 Kafka·RabbitMQ 같은 메시징 시스템도 포함 가능\n")
	}
	sb.WriteString("- 계층형 주제 3-5개: \"도메인 → 중분류 → 구체 개념\" 형식\n")
	sb.WriteString("- 예: \"데이터베이스 → Vector DB → HNSW 인덱스 구조\"\n\n")
	sb.WriteString("JSON: {\"domain\":\"...\",\"topics\":[\"...\"]}")
	return sb.String()
}
```

- [ ] **Step 4: 테스트 실행해서 통과 확인**

Run: `go test ./internal/devdigest/ -run TestBuildTopicPrompt -v -race`
Expected: 2개 PASS

- [ ] **Step 5: 전체 devdigest 테스트 + 빌드**

Run: `go test ./internal/devdigest/ -race && go build ./... && go vet ./internal/devdigest/`
Expected: PASS, 빌드/vet 클린

- [ ] **Step 6: 커밋**

```bash
git add internal/devdigest/digest.go internal/devdigest/digest_test.go
git commit -m "feat(devdigest): GenerateTopics — 공부 주제 전용 생성(대화형 재요청용)"
```

---

### Task 2: agent.StudyTools — suggest_study_topics 도구

**Files:**
- Create: `internal/agent/study_tools.go`
- Create: `internal/agent/study_tools_test.go`

**Interfaces:**
- Consumes:
  - `devdigest.TopicResult{Domain string, Topics []string}` (Task 1)
  - 기존 `Tool` 구조체, `objSchema`, `strSchema`, `strArg` (tools.go)
- Produces:
  - `agent.StudyTopicGenerator` 인터페이스: `GenerateTopics(ctx context.Context, domain string) (devdigest.TopicResult, error)`
  - `func StudyTools(gen StudyTopicGenerator) []Tool`

- [ ] **Step 1: study_tools_test.go 작성**

```go
package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

type fakeStudyGen struct {
	gotDomain string
	result    devdigest.TopicResult
	err       error
}

func (f *fakeStudyGen) GenerateTopics(_ context.Context, domain string) (devdigest.TopicResult, error) {
	f.gotDomain = domain
	return f.result, f.err
}

// studyTool 은 StudyTools 의 단일 도구를 꺼낸다.
func studyTool(gen StudyTopicGenerator) Tool {
	return StudyTools(gen)[0]
}

func TestSuggestStudyTopics_passesDomainAndFormats(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{result: devdigest.TopicResult{
		Domain: "운영체제",
		Topics: []string{"운영체제 → 스케줄링 → CFS", "운영체제 → 메모리 → 페이지 폴트"},
	}}
	tool := studyTool(gen)
	out, err := tool.Run(context.Background(), map[string]any{"domain": "운영체제"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gen.gotDomain != "운영체제" {
		t.Fatalf("domain 전달 실패: %q", gen.gotDomain)
	}
	if !strings.Contains(out, "운영체제") || !strings.Contains(out, "CFS") || !strings.Contains(out, "페이지 폴트") {
		t.Fatalf("주제 포맷 누락: %q", out)
	}
	if !strings.Contains(out, "공부 주제") {
		t.Fatalf("헤더 없음: %q", out)
	}
}

func TestSuggestStudyTopics_emptyDomain(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{result: devdigest.TopicResult{Domain: "AI", Topics: []string{"AI → LLM → RAG"}}}
	tool := studyTool(gen)
	out, err := tool.Run(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gen.gotDomain != "" {
		t.Fatalf("domain 미지정 시 빈 문자열 기대: %q", gen.gotDomain)
	}
	if !strings.Contains(out, "RAG") {
		t.Fatalf("주제 누락: %q", out)
	}
}

func TestSuggestStudyTopics_errorPropagates(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{err: fmt.Errorf("gemini 실패")}
	tool := studyTool(gen)
	_, err := tool.Run(context.Background(), map[string]any{"domain": "DB"})
	if err == nil {
		t.Fatal("generator error 시 도구도 error 기대")
	}
}

func TestStudyTools_isReadOnly(t *testing.T) {
	t.Parallel()
	tool := studyTool(&fakeStudyGen{})
	if tool.Write {
		t.Fatal("suggest_study_topics 는 읽기 도구여야 한다")
	}
	if tool.Decl.Name != "suggest_study_topics" {
		t.Fatalf("도구 이름=%q", tool.Decl.Name)
	}
}
```

- [ ] **Step 2: 테스트 실행해서 실패 확인**

Run: `go test ./internal/agent/ -run TestSuggestStudyTopics -v 2>&1 | head -15`
Expected: FAIL (컴파일 에러 — `StudyTools`/`StudyTopicGenerator` 미정의)

- [ ] **Step 3: study_tools.go 구현**

```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

// StudyTopicGenerator 는 공부 주제를 생성하는 능력이다(테스트에서 fake 주입).
type StudyTopicGenerator interface {
	GenerateTopics(ctx context.Context, domain string) (devdigest.TopicResult, error)
}

type studyTools struct {
	gen StudyTopicGenerator
}

// StudyTools 는 공부 주제 추천 도구 목록을 만든다(읽기형).
func StudyTools(gen StudyTopicGenerator) []Tool {
	s := studyTools{gen: gen}
	return []Tool{s.suggestStudyTopics()}
}

func (s studyTools) suggestStudyTopics() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "suggest_study_topics",
			Description: "개발 공부 주제를 추천한다. 사용자가 '다른 공부 주제', '운영체제 주제', 'DB 다른 거' 등을 요청할 때 호출한다. domain 에 특정 도메인/주제를 넣으면 그 주제로, 비우면 임의 도메인으로 생성한다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"domain": strSchema("공부 도메인 또는 세부 주제. 예: 운영체제, 데이터베이스, 쿠버네티스. 미지정 시 임의 선택."),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			result, err := s.gen.GenerateTopics(ctx, strArg(args, "domain"))
			if err != nil {
				return "", err
			}
			return formatTopics(result), nil
		},
	}
}

// formatTopics 는 공부 주제를 Slack 텍스트로 만든다(아침 digest 공부주제 섹션과 동일 형식).
func formatTopics(r devdigest.TopicResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📚 *공부 주제*  _(도메인: %s)_\n", r.Domain))
	for _, topic := range r.Topics {
		sb.WriteString("• " + topic + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
```

- [ ] **Step 4: 테스트 실행해서 통과 확인**

Run: `go test ./internal/agent/ -run "TestSuggestStudyTopics|TestStudyTools" -v -race`
Expected: 4개 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/agent/study_tools.go internal/agent/study_tools_test.go
git commit -m "feat(agent): suggest_study_topics 도구 — 대화로 공부 주제 재생성"
```

---

### Task 3: 배선 + 시스템 프롬프트 힌트

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/agent/agent.go`

**Interfaces:**
- Consumes: `agent.StudyTools` (Task 2), `devdigest.NewGenerator` (기존)

- [ ] **Step 1: main.go 에 도구 배선**

`cmd/server/main.go` 에서, `tools` 에 KnowledgeTools 를 append 하는 줄 다음(IngestTools append 전후 무방, Todoist 블록 앞)에 추가:

```go
	// 공부 주제 추천 도구(대화형 재요청). 읽기형이라 항상 등록.
	studyGen := devdigest.NewGenerator(geminiClient)
	tools = append(tools, agent.StudyTools(studyGen)...)
```

`devdigest` 가 이미 import 되어 있지 않으면 import 블록에 추가:
```go
	"github.com/Jongseong0111/jarvis/internal/devdigest"
```
(주의: 이 import 는 dev-digest 기능에서 이미 추가되어 있을 수 있다. 중복 추가하지 말 것 — 빌드 시 `imported and not used` 또는 중복 에러로 확인.)

- [ ] **Step 2: agent.go 시스템 프롬프트에 힌트 추가**

`internal/agent/agent.go` 의 `DefaultSystemPrompt` 마지막 규칙(start_concept_ingest 관련 줄) 다음에 한 줄 추가. 백틱 문자열 안이므로 마지막 줄 끝의 `` ` `` 직전에 삽입한다:

```
- 사용자가 "공부 주제 추천/다른 거/특정 도메인(운영체제 등) 주제"를 요청하면 suggest_study_topics 를 호출한다. 특정 도메인을 말하면 domain 인자에 넣고, 아니면 비운다.
```

즉, 다음과 같이 된다(끝부분):
```go
- 사용자가 명시적으로 "개념 정리해줘", "지식 정리해줘" 등을 요청하면 start_concept_ingest 를 호출한다. source_path 가 언급되지 않으면 직전 대화에서 저장된 경로를 쓴다.
- 사용자가 "공부 주제 추천/다른 거/특정 도메인(운영체제 등) 주제"를 요청하면 suggest_study_topics 를 호출한다. 특정 도메인을 말하면 domain 인자에 넣고, 아니면 비운다.`
```

- [ ] **Step 3: 빌드 + 전체 테스트**

Run: `go build ./... && go test ./... -race`
Expected: 빌드 클린, 전 패키지 PASS

- [ ] **Step 4: 커밋**

```bash
git add cmd/server/main.go internal/agent/agent.go
git commit -m "feat(server): suggest_study_topics 도구 배선 + 시스템 프롬프트 힌트"
```

---

## 검증 (전체 완료 후, 컨트롤러가 수행)

라이브 검증은 서버 재시작이 필요하므로 컨트롤러가 별도로 수행한다(서브에이전트 범위 밖):
- `go build -o bin/jarvis ./cmd/server` → 재시작
- Slack에서 "운영체제 공부 주제 줘", "다른 거 줄래?" 테스트
