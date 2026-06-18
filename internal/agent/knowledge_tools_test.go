package agent

import (
	"context"
	"strings"
	"testing"
)

type fakeKnowledge struct {
	title, summary                     string
	savedTitle, savedURL, savedContent string
	path                               string
}

func (f *fakeKnowledge) Summarize(_ context.Context, _ string) (string, string, error) {
	return f.title, f.summary, nil
}
func (f *fakeKnowledge) SaveSource(_ context.Context, title, url, content string) (string, error) {
	f.savedTitle, f.savedURL, f.savedContent = title, url, content
	return f.path, nil
}

func toolByName(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Decl.Name == name {
			return t
		}
	}
	return Tool{}
}

func TestKnowledge_summarizeTool_returnsSummaryNoSave(t *testing.T) {
	t.Parallel()
	fk := &fakeKnowledge{title: "고랭 장점 설명", summary: "## 핵심\n- 고루틴"}
	tool := toolByName(KnowledgeTools(fk), "summarize_chatgpt_share")
	out, err := tool.Run(context.Background(), map[string]any{"url": "https://chatgpt.com/share/x"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "고랭 장점 설명") || !strings.Contains(out, "고루틴") {
		t.Fatalf("요약 응답 = %q", out)
	}
	if fk.savedContent != "" {
		t.Fatal("summarize 는 저장하면 안 됨")
	}
}

func TestKnowledge_saveTool_passesContent(t *testing.T) {
	t.Parallel()
	fk := &fakeKnowledge{path: "/kb/sources/conversation/2026-06-18-고랭-장점-설명.md"}
	tool := toolByName(KnowledgeTools(fk), "save_kb_source")
	out, err := tool.Run(context.Background(), map[string]any{
		"title": "고랭 장점 설명", "url": "https://chatgpt.com/share/x", "content": "## 핵심(수정됨)\n- 채널",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fk.savedContent != "## 핵심(수정됨)\n- 채널" || fk.savedTitle != "고랭 장점 설명" {
		t.Fatalf("저장 인자 = %q / %q", fk.savedTitle, fk.savedContent)
	}
	if !strings.Contains(out, fk.path) {
		t.Fatalf("저장 응답에 경로 없음: %q", out)
	}
}
