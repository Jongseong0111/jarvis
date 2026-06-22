package devdigest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/internal/gemini"
	"github.com/Jongseong0111/jarvis/internal/usage"
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

// TopicResult 는 공부 주제 생성 결과다(뉴스 없이 주제만).
type TopicResult struct {
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

// maxGeekNews 는 결과에 허용하는 GeekNews 최대 개수다(나머지는 다른 출처로 섞기 위함).
const maxGeekNews = 2

// Generate 는 뉴스 후보 목록을 Gemini 에 보내 선별·요약 + 공부주제를 생성한다.
// GeekNews 는 후보 풀에 그대로 두고 프롬프트로 "1~2개만 섞어라"를 지시하되,
// 모델이 치우치면 코드가 GeekNews 를 최대 maxGeekNews 로 자르고, 0개면 1개 주입한다.
func (g *GeminiGenerator) Generate(ctx context.Context, items []NewsItem) (DigestResult, error) {
	geek := geekNewsItems(items)
	ctx = usage.WithFeature(ctx, "digest")
	raw, err := g.client.GenerateText(ctx, systemPrompt, buildPrompt(items, len(geek) > 0))
	if err != nil {
		return DigestResult{}, fmt.Errorf("gemini 다이제스트 생성 실패: %w", err)
	}
	result, err := parseResponse(raw)
	if err != nil {
		return DigestResult{}, err
	}
	return balanceNews(result, items), nil
}

// GenerateTopics 는 공부 주제만 생성한다(대화형 재요청용).
// requestedDomain 이 비면 모델이 11개 도메인 중 하나를 선택하고,
// 지정되면 그 도메인(또는 더 구체적인 세부 주제 힌트)으로 계층형 주제를 만든다.
func (g *GeminiGenerator) GenerateTopics(ctx context.Context, requestedDomain string) (TopicResult, error) {
	ctx = usage.WithFeature(ctx, "digest")
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

// geekNewsItems 는 후보 중 GeekNews 출처 항목만 추린다(피드 순서 유지).
func geekNewsItems(items []NewsItem) []NewsItem {
	var out []NewsItem
	for _, it := range items {
		if it.Source == "GeekNews" {
			out = append(out, it)
		}
	}
	return out
}

// balanceNews 는 결과의 GeekNews 비중을 조정한다.
// GeekNews 가 maxGeekNews 를 넘으면 초과분을 제거하고(다른 출처 우선),
// 하나도 없으면 첫 GeekNews 를 맨 앞에 주입한다.
func balanceNews(result DigestResult, items []NewsItem) DigestResult {
	geek := geekNewsItems(items)
	if len(geek) == 0 {
		return result
	}
	srcByURL := make(map[string]string, len(items))
	for _, it := range items {
		srcByURL[it.URL] = it.Source
	}

	var kept []NewsResult
	geekCount := 0
	for _, n := range result.News {
		if srcByURL[n.URL] == "GeekNews" {
			if geekCount >= maxGeekNews {
				continue // 초과 GeekNews 제거
			}
			geekCount++
		}
		kept = append(kept, n)
	}

	if geekCount == 0 {
		forced := geek[0]
		summary := strings.TrimSpace(forced.Desc)
		if summary == "" {
			summary = "GeekNews 인기 글"
		}
		kept = append([]NewsResult{{Title: forced.Title, URL: forced.URL, Summary: summary}}, kept...)
	}

	result.News = kept
	return result
}

const systemPrompt = `너는 개발자를 위한 아침 다이제스트를 만드는 어시스턴트다. 반드시 JSON 으로만 응답하라. 마크다운 코드블록 없이 순수 JSON 만 출력하라.`

// buildPrompt 는 전체 후보를 출처 라벨과 함께 나열하고, GeekNews 가 있으면
// "최소 1개 반드시 포함" 지시를 더한다(모델이 가장 흥미로운 GeekNews 를 고르게 함).
func buildPrompt(items []NewsItem, hasGeekNews bool) string {
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
	if hasGeekNews {
		sb.WriteString("   - [GeekNews] 1~2개(가장 흥미로운 것)와 다른 출처 3~4개를 섞어라. 한 출처에 치우치지 마라\n")
	}
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
