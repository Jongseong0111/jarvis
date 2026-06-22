package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

type fakeStudyGen struct {
	gotDomain string
	result    devdigest.TopicResult
	err       error
}

func (f *fakeStudyGen) GenerateTopics(_ context.Context, domain string) (devdigest.TopicResult, error) {
	f.gotDomain = domain
	return f.result, f.err
}

// studyTool 은 StudyTools 의 단일 도구를 꺼낸다.
func studyTool(gen StudyTopicGenerator) Tool {
	return StudyTools(gen)[0]
}

func TestSuggestStudyTopics_passesDomainAndFormats(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{result: devdigest.TopicResult{
		Domain: "운영체제",
		Topics: []string{"운영체제 → 스케줄링 → CFS", "운영체제 → 메모리 → 페이지 폴트"},
	}}
	tool := studyTool(gen)
	out, err := tool.Run(context.Background(), map[string]any{"domain": "운영체제"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gen.gotDomain != "운영체제" {
		t.Fatalf("domain 전달 실패: %q", gen.gotDomain)
	}
	if !strings.Contains(out, "운영체제") || !strings.Contains(out, "CFS") || !strings.Contains(out, "페이지 폴트") {
		t.Fatalf("주제 포맷 누락: %q", out)
	}
	if !strings.Contains(out, "공부 주제") {
		t.Fatalf("헤더 없음: %q", out)
	}
}

func TestSuggestStudyTopics_emptyDomain(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{result: devdigest.TopicResult{Domain: "AI", Topics: []string{"AI → LLM → RAG"}}}
	tool := studyTool(gen)
	out, err := tool.Run(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gen.gotDomain != "" {
		t.Fatalf("domain 미지정 시 빈 문자열 기대: %q", gen.gotDomain)
	}
	if !strings.Contains(out, "RAG") {
		t.Fatalf("주제 누락: %q", out)
	}
}

func TestSuggestStudyTopics_errorPropagates(t *testing.T) {
	t.Parallel()
	gen := &fakeStudyGen{err: fmt.Errorf("gemini 실패")}
	tool := studyTool(gen)
	_, err := tool.Run(context.Background(), map[string]any{"domain": "DB"})
	if err == nil {
		t.Fatal("generator error 시 도구도 error 기대")
	}
}

func TestStudyTools_isReadOnly(t *testing.T) {
	t.Parallel()
	tool := studyTool(&fakeStudyGen{})
	if tool.Write {
		t.Fatal("suggest_study_topics 는 읽기 도구여야 한다")
	}
	if tool.Decl.Name != "suggest_study_topics" {
		t.Fatalf("도구 이름=%q", tool.Decl.Name)
	}
}
