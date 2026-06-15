package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/gemini"
)

// Extracted 는 집 정리 메시지에서 추출한 구조화 결과다.
type Extracted struct {
	Action   string `json:"action"`   // "add" / "search"
	Item     string `json:"item"`     // 물건 이름
	Category string `json:"category"` // 제공된 목록 중 하나, 없으면 ""
	Location string `json:"location"` // 제공된 목록 중 하나, 없으면 ""
	Quantity *int   `json:"quantity"` // 수량, 없으면 null
}

// Extractor 는 텍스트와 현재 장소/카테고리 목록으로 구조화 결과를 만든다.
type Extractor interface {
	Extract(ctx context.Context, text string, locations, categories []string) (Extracted, error)
}

// GeminiExtractor 는 공유 Gemini 클라이언트로 JSON 구조화 추출을 수행한다.
type GeminiExtractor struct {
	client *gemini.Client
}

// NewGeminiExtractor 는 GeminiExtractor 를 생성한다.
func NewGeminiExtractor(client *gemini.Client) GeminiExtractor {
	return GeminiExtractor{client: client}
}

// Extract 는 텍스트를 Extracted 로 변환한다. category/location 은 제공 목록에서만 고른다.
func (e GeminiExtractor) Extract(ctx context.Context, text string, locations, categories []string) (Extracted, error) {
	out, err := e.client.GenerateJSON(ctx, buildExtractPrompt(text, locations, categories), extractSchema())
	if err != nil {
		return Extracted{}, fmt.Errorf("집정리 추출 호출 실패: %w", err)
	}
	var ex Extracted
	if err := json.Unmarshal([]byte(out), &ex); err != nil {
		return Extracted{}, fmt.Errorf("집정리 추출 응답 파싱 실패: %w (%q)", err, out)
	}
	ex.Item = strings.TrimSpace(ex.Item)
	ex.Category = strings.TrimSpace(ex.Category)
	ex.Location = strings.TrimSpace(ex.Location)
	return ex, nil
}

// buildExtractPrompt 는 추출 지시문 + 현재 장소/카테고리 목록 + 사용자 텍스트로 프롬프트를 만든다. (순수 함수)
func buildExtractPrompt(text string, locations, categories []string) string {
	var b strings.Builder
	b.WriteString("너는 집 정리 비서다. 사용자 메시지에서 다음을 추출해라.\n")
	b.WriteString("- action: 물건을 어딘가에 넣었으면 \"add\", 위치를 물어보면 \"search\"\n")
	b.WriteString("- item: 물건 이름\n")
	b.WriteString("- location: 아래 '장소 목록' 중에서 가장 맞는 것 하나. 없으면 빈 문자열\n")
	b.WriteString("- category: 아래 '카테고리 목록' 중에서 가장 맞는 것 하나. 없으면 빈 문자열\n")
	b.WriteString("- quantity: 수량 숫자. 없으면 null\n\n")
	b.WriteString("장소 목록: ")
	b.WriteString(strings.Join(locations, ", "))
	b.WriteString("\n카테고리 목록: ")
	b.WriteString(strings.Join(categories, ", "))
	b.WriteString("\n\nlocation/category 는 반드시 위 목록의 값 그대로 쓰거나 빈 문자열이어야 한다.\n\n")
	fmt.Fprintf(&b, "사용자 메시지: %s", text)
	return b.String()
}

// extractSchema 는 추출 결과 JSON 스키마다. (순수 함수)
func extractSchema() *genai.Schema {
	nullable := true
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"action":   {Type: genai.TypeString, Enum: []string{"add", "search"}},
			"item":     {Type: genai.TypeString},
			"category": {Type: genai.TypeString},
			"location": {Type: genai.TypeString},
			"quantity": {Type: genai.TypeInteger, Nullable: &nullable},
		},
		Required: []string{"action", "item"},
	}
}
