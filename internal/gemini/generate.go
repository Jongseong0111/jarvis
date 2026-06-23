package gemini

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// GenerateText 는 도구 없이 일반 텍스트를 생성한다(요약 등). 온도 0(결정론적).
func (c *Client) GenerateText(ctx context.Context, system, user string) (string, error) {
	return c.GenerateTextTemp(ctx, system, user, 0)
}

// GenerateTextTemp 는 온도를 지정해 텍스트를 생성한다.
// 공부 주제처럼 다양성이 필요한 생성에는 0 보다 큰 값을 쓴다.
func (c *Client) GenerateTextTemp(ctx context.Context, system, user string, temperature float32) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	temp := temperature
	thinkBudget := int32(0)
	cfg := &genai.GenerateContentConfig{
		Temperature:    &temp,
		ThinkingConfig: &genai.ThinkingConfig{ThinkingBudget: &thinkBudget},
	}
	if system != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}

	resp, err := client.Models.GenerateContent(ctx, c.model, genai.Text(user), cfg)
	if err != nil {
		return "", fmt.Errorf("gemini 생성 실패: %w", err)
	}
	c.record(ctx, resp, "text")
	out := strings.TrimSpace(resp.Text())
	if out == "" {
		return "", fmt.Errorf("gemini 빈 응답")
	}
	return out, nil
}
