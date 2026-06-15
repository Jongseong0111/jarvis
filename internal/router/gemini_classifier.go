package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

const (
	defaultModel     = "gemini-2.5-flash-lite"
	classifyTimeout  = 15 * time.Second
	classifyEnumMIME = "text/x.enum" // Gemini enum 응답 MIME 타입
)

// intentDescriptions 는 분류 프롬프트에 노출할 intent별 한국어 설명이다.
// (todo/scheduler 는 대응 Worker 가 없어 제외한다.)
var intentDescriptions = []struct {
	intent domain.Intent
	desc   string
}{
	{domain.IntentHomeSearch, "집안 물건의 위치/수량을 찾거나 조회"},
	{domain.IntentHomeAdd, "집안 물건을 새로 추가(어디에 무엇을 넣었다)"},
	{domain.IntentHomeUpdate, "집안 물건의 위치나 수량을 수정"},
	{domain.IntentHomeDelete, "집안 물건을 삭제/뺐다"},
	{domain.IntentKnowledgeUpdate, "개발 개념·에러·운영 경험·공부 내용을 지식 저장소에 정리/저장"},
	{domain.IntentKnowledgeSearch, "지식 저장소에서 정리된 내용을 검색/조회"},
	{domain.IntentKnowledgeReview, "지식 저장소의 변경안을 리뷰"},
	{domain.IntentSystemHelp, "이 에이전트가 뭘 할 수 있는지 도움말 요청"},
	{domain.IntentUnknown, "위 어디에도 해당하지 않거나 모호한 경우"},
}

// GeminiClassifier 는 Gemini API 로 텍스트를 intent enum 으로 분류한다.
// domain.Classifier 를 구현한다.
type GeminiClassifier struct {
	apiKey string
	model  string
}

// NewGeminiClassifier 는 GeminiClassifier 를 생성한다. model 이 비면 기본 모델을 쓴다.
func NewGeminiClassifier(apiKey, model string) GeminiClassifier {
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}
	return GeminiClassifier{apiKey: apiKey, model: model}
}

// Classify 는 Gemini structured output(enum 제약)으로 텍스트를 intent 로 분류한다.
// 호출 실패는 error 로 반환하고, 비유효 응답은 system.unknown 으로 흡수한다.
func (c GeminiClassifier) Classify(ctx context.Context, text string) (domain.Intent, error) {
	ctx, cancel := context.WithTimeout(ctx, classifyTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	temp := float32(0)
	resp, err := client.Models.GenerateContent(
		ctx,
		c.model,
		genai.Text(buildClassifyPrompt(text)),
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: classifyEnumMIME,
			ResponseSchema: &genai.Schema{
				Type: genai.TypeString,
				Enum: enumValues(),
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("intent 분류 호출 실패: %w", err)
	}
	return validateIntent(resp.Text()), nil
}

// buildClassifyPrompt 는 분류 지시문 + intent 설명 + 사용자 텍스트로 프롬프트를 만든다. (순수 함수)
func buildClassifyPrompt(text string) string {
	var b strings.Builder
	b.WriteString("너는 개인 에이전트의 의도 분류기다. 아래 사용자 메시지가 어떤 작업을 의도하는지 ")
	b.WriteString("정확히 하나의 intent 로 분류해라.\n\n")
	b.WriteString("가능한 intent:\n")
	for _, d := range intentDescriptions {
		fmt.Fprintf(&b, "- %s: %s\n", d.intent, d.desc)
	}
	b.WriteString("\n잘 모르겠거나 모호하면 system.unknown 을 골라라.\n\n")
	fmt.Fprintf(&b, "사용자 메시지: %s", text)
	return b.String()
}

// enumValues 는 Gemini enum 제약에 넣을 유효 intent 문자열 목록을 반환한다. (순수 함수)
func enumValues() []string {
	all := domain.AllIntents()
	out := make([]string, len(all))
	for i, in := range all {
		out[i] = string(in)
	}
	return out
}

// validateIntent 는 모델 응답 문자열을 유효 Intent 로 검증한다.
// 알 수 없는 값이면 system.unknown 으로 흡수한다. (순수 함수)
func validateIntent(s string) domain.Intent {
	got := domain.Intent(strings.TrimSpace(s))
	for _, valid := range domain.AllIntents() {
		if got == valid {
			return got
		}
	}
	return domain.IntentUnknown
}
