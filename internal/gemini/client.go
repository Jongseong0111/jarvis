// Package gemini 는 Gemini(genai) 호출을 감싸는 공유 클라이언트다.
package gemini

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

// DefaultModel 은 model 미지정 시 사용하는 기본 모델이다.
const DefaultModel = "gemini-2.5-flash-lite"

const requestTimeout = 30 * time.Second

// Client 는 Gemini API 호출을 감싼다.
type Client struct {
	apiKey string
	model  string
}

// New 는 Client 를 생성한다. model 이 비면 기본 모델을 쓴다.
func New(apiKey, model string) *Client {
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	return &Client{apiKey: apiKey, model: model}
}

// GenerateWithTools 는 도구(function declarations)와 함께 생성한다.
// 응답에는 텍스트 또는 FunctionCall 파트가 담긴다(에이전트 루프가 해석).
func (c *Client) GenerateWithTools(ctx context.Context, contents []*genai.Content, tools []*genai.Tool, systemPrompt string) (*genai.GenerateContentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	temp := float32(0)
	cfg := &genai.GenerateContentConfig{
		Temperature: &temp,
		Tools:       tools,
	}
	if systemPrompt != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: systemPrompt}}}
	}

	resp, err := client.Models.GenerateContent(ctx, c.model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini 생성 실패: %w", err)
	}
	return resp, nil
}
