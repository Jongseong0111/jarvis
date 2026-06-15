// Package gemini 는 Gemini(genai) 호출을 감싸는 공유 클라이언트다.
// 의도 분류(enum)와 구조화 추출(JSON)에서 공용으로 쓴다.
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

const requestTimeout = 15 * time.Second

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

// GenerateEnum 은 출력을 enum 값 중 하나로 제약해 생성한다.
func (c *Client) GenerateEnum(ctx context.Context, prompt string, enum []string) (string, error) {
	return c.generate(ctx, prompt, "text/x.enum", &genai.Schema{Type: genai.TypeString, Enum: enum})
}

// GenerateJSON 은 출력을 주어진 schema 의 JSON 으로 제약해 생성한다.
func (c *Client) GenerateJSON(ctx context.Context, prompt string, schema *genai.Schema) (string, error) {
	return c.generate(ctx, prompt, "application/json", schema)
}

func (c *Client) generate(ctx context.Context, prompt, mime string, schema *genai.Schema) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
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
		genai.Text(prompt),
		&genai.GenerateContentConfig{
			Temperature:      &temp,
			ResponseMIMEType: mime,
			ResponseSchema:   schema,
		},
	)
	if err != nil {
		return "", fmt.Errorf("gemini 생성 실패: %w", err)
	}
	return resp.Text(), nil
}
