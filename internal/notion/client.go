package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL = "https://api.notion.com"
	notionVersion  = "2022-06-28"
	requestTimeout = 15 * time.Second
)

// Client 는 Notion REST API 클라이언트다.
type Client struct {
	apiKey  string
	http    *http.Client
	baseURL string
}

// New 는 Client 를 생성한다.
func New(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		http:    &http.Client{Timeout: requestTimeout},
		baseURL: defaultBaseURL,
	}
}

// queryResponse 는 database query 응답이다.
type queryResponse struct {
	Results    []Page `json:"results"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

// QueryDatabase 는 DB 를 조회해 모든 페이지를 반환한다(has_more 시 커서로 이어 조회).
// body 는 filter/sorts 등을 담은 맵이며 nil 이면 전체 조회한다.
func (c *Client) QueryDatabase(ctx context.Context, dbID string, body map[string]any) ([]Page, error) {
	if body == nil {
		body = map[string]any{}
	}
	var pages []Page
	cursor := ""
	for {
		req := cloneMap(body)
		req["page_size"] = 100
		if cursor != "" {
			req["start_cursor"] = cursor
		}
		var out queryResponse
		if err := c.post(ctx, "/v1/databases/"+dbID+"/query", req, &out); err != nil {
			return nil, err
		}
		pages = append(pages, out.Results...)
		if !out.HasMore || out.NextCursor == "" {
			break
		}
		cursor = out.NextCursor
	}
	return pages, nil
}

// createResponse 는 page 생성 응답이다.
type createResponse struct {
	ID string `json:"id"`
}

// CreatePage 는 dbID 를 부모로 page 를 생성하고 생성된 page ID 를 반환한다.
func (c *Client) CreatePage(ctx context.Context, dbID string, props map[string]any) (string, error) {
	body := map[string]any{
		"parent":     map[string]any{"database_id": dbID},
		"properties": props,
	}
	var out createResponse
	if err := c.post(ctx, "/v1/pages", body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// UpdatePage 는 page 의 properties 를 갱신한다.
func (c *Client) UpdatePage(ctx context.Context, pageID string, props map[string]any) error {
	var out createResponse
	return c.patch(ctx, "/v1/pages/"+pageID, map[string]any{"properties": props}, &out)
}

// ArchivePage 는 page 를 보관(삭제) 처리한다.
func (c *Client) ArchivePage(ctx context.Context, pageID string) error {
	var out createResponse
	return c.patch(ctx, "/v1/pages/"+pageID, map[string]any{"archived": true}, &out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *Client) patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("notion 요청 직렬화 실패: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("notion 요청 생성 실패: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Notion-Version", notionVersion)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("notion 요청 실패: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notion API 오류(%d): %s", resp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("notion 응답 파싱 실패: %w", err)
	}
	return nil
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m)+2)
	for k, v := range m {
		out[k] = v
	}
	return out
}
