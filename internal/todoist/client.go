package todoist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBase = "https://api.todoist.com/rest/v2"

// Client 는 Todoist REST 클라이언트다.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New 는 기본 base URL 로 클라이언트를 만든다.
func New(token string) *Client { return NewWithBase(token, defaultBase) }

// NewWithBase 는 base URL 을 주입한다(테스트용).
func NewWithBase(token, baseURL string) *Client {
	return &Client{token: token, baseURL: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// apiTask 는 Todoist 응답 형태(내부 디코딩용).
type apiTask struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
	Due       *struct {
		String string `json:"string"`
		Date   string `json:"date"`
	} `json:"due"`
}

func (a apiTask) toTask() Task {
	due := ""
	if a.Due != nil {
		if a.Due.String != "" {
			due = a.Due.String
		} else {
			due = a.Due.Date
		}
	}
	return Task{ID: a.ID, Content: a.Content, Due: due, Project: a.ProjectID, URL: a.URL}
}

// do 는 요청을 보내고 2xx 가 아니면 에러를 만든다. respBody 가 non-nil 이면 JSON 디코딩.
func (c *Client) do(ctx context.Context, method, path string, reqBody, respBody any) error {
	var r io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("요청 직렬화: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return fmt.Errorf("요청 생성: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("요청 전송: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("todoist %s %s: %d %s", method, path, resp.StatusCode, string(body))
	}
	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("응답 파싱: %w", err)
		}
	}
	return nil
}

// ListTasks 는 필터(예: "today | overdue")에 맞는 할일을 조회한다.
func (c *Client) ListTasks(ctx context.Context, filter string) ([]Task, error) {
	path := "/tasks"
	if filter != "" {
		path += "?filter=" + url.QueryEscape(filter)
	}
	var raw []apiTask
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	tasks := make([]Task, len(raw))
	for i, a := range raw {
		tasks[i] = a.toTask()
	}
	return tasks, nil
}

// AddTask 는 할일을 추가한다. due/project 는 빈 문자열이면 생략.
func (c *Client) AddTask(ctx context.Context, content, due, project string) (Task, error) {
	body := map[string]any{"content": content}
	if due != "" {
		body["due_string"] = due
	}
	if project != "" {
		body["project_id"] = project
	}
	var raw apiTask
	if err := c.do(ctx, http.MethodPost, "/tasks", body, &raw); err != nil {
		return Task{}, err
	}
	return raw.toTask(), nil
}

// CompleteTask 는 할일을 완료 처리한다.
func (c *Client) CompleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/tasks/"+id+"/close", nil, nil)
}

// UpdateTask 는 내용/마감을 수정한다(빈 값은 변경 안 함).
func (c *Client) UpdateTask(ctx context.Context, id, content, due string) error {
	body := map[string]any{}
	if content != "" {
		body["content"] = content
	}
	if due != "" {
		body["due_string"] = due
	}
	return c.do(ctx, http.MethodPost, "/tasks/"+id, body, nil)
}

// DeleteTask 는 할일을 삭제한다.
func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/tasks/"+id, nil, nil)
}
