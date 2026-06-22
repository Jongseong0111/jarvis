package devdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/internal/gemini"
)

// domains 는 공부 주제 도메인 목록이다. Gemini 가 매일 하나를 선택한다.
var domains = []string{
	"언어", "웹·백엔드", "데이터베이스", "인프라",
	"데이터", "운영체제", "네트워크",
	"자료구조·알고리즘", "개발도구", "AI", "기타",
}

// NewsResult 는 Gemini 가 선별한 뉴스 기사 하나다.
type NewsResult struct {
	Title   string
	URL     string
	Summary string
}

// DigestResult 는 Gemini 가 생성한 뉴스+공부주제 다이제스트다.
type DigestResult struct {
	News   []NewsResult
	Domain string
	Topics []string
}

// Generator 는 뉴스 아이템에서 다이제스트를 생성하는 인터페이스다.
type Generator interface {
	Generate(ctx context.Context, items []NewsItem) (DigestResult, error)
}

// GeminiGenerator 는 Gemini 를 사용해 다이제스트를 생성한다.
type GeminiGenerator struct {
	client *gemini.Client
}

// NewGenerator 는 GeminiGenerator 를 생성한다.
func NewGenerator(client *gemini.Client) *GeminiGenerator {
	return &GeminiGenerator{client: client}
}

// Generate 는 뉴스 후보 목록을 Gemini 에 보내 선별·요약 + 공부주제를 생성한다.
func (g *GeminiGenerator) Generate(ctx context.Context, items []NewsItem) (DigestResult, error) {
	raw, err := g.client.GenerateText(ctx, systemPrompt, buildPrompt(items))
	if err != nil {
		return DigestResult{}, fmt.Errorf("gemini 다이제스트 생성 실패: %w", err)
	}
	return parseResponse(raw)
}

const systemPrompt = `너는 개발자를 위한 아침 다이제스트를 만드는 어시스턴트다. 반드시 JSON 으로만 응답하라. 마크다운 코드블록 없이 순수 JSON 만 출력하라.`

func buildPrompt(items []NewsItem) string {
	var sb strings.Builder
	sb.WriteString("[뉴스 후보 목록]\n")
	for i, it := range items {
		source := it.Source
		if source == "" {
			source = "기타"
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s | %s | %s\n", i+1, source, it.Title, it.URL, it.Desc))
	}
	sb.WriteString("\n[작업]\n")
	sb.WriteString("1. 위 목록에서 개발자에게 가장 흥미로운 항목 3-5개를 골라라.\n")
	sb.WriteString("   - 실제 기술 내용 우선 (채용/마케팅/이벤트 제외)\n")
	sb.WriteString("   - [GeekNews] 출처 항목이 후보에 있으면 그중 최소 1-2개를 반드시 포함하라\n")
	sb.WriteString("   - 각 항목: title(원문 유지), url(원문), summary(한국어 한줄 요약)\n\n")
	sb.WriteString("2. 오늘의 개발 공부 주제를 생성하라.\n")
	sb.WriteString("   - 아래 도메인 중 하나 선택: " + strings.Join(domains, " / ") + "\n")
	sb.WriteString("   - 인프라 선택 시 Kafka·RabbitMQ 같은 메시징 시스템도 포함 가능\n")
	sb.WriteString("   - 계층형 주제 3-5개: \"도메인 → 중분류 → 구체 개념\" 형식\n")
	sb.WriteString("   - 예: \"데이터베이스 → Vector DB → HNSW 인덱스 구조\"\n\n")
	sb.WriteString("JSON: {\"news\":[{\"title\":\"...\",\"url\":\"...\",\"summary\":\"...\"}],\"domain\":\"...\",\"topics\":[\"...\"]}")
	return sb.String()
}

type digestJSON struct {
	News []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Summary string `json:"summary"`
	} `json:"news"`
	Domain string   `json:"domain"`
	Topics []string `json:"topics"`
}

func parseResponse(raw string) (DigestResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var dj digestJSON
	if err := json.Unmarshal([]byte(raw), &dj); err != nil {
		return DigestResult{}, fmt.Errorf("응답 JSON 파싱 실패: %w", err)
	}

	result := DigestResult{Domain: dj.Domain, Topics: dj.Topics}
	for _, n := range dj.News {
		result.News = append(result.News, NewsResult{Title: n.Title, URL: n.URL, Summary: n.Summary})
	}
	return result, nil
}
