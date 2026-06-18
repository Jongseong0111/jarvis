package knowledge

import (
	"context"
	"strings"
	"time"
)

// summarizer 는 요약에 쓰는 텍스트 생성 능력이다(*gemini.Client 가 만족).
type summarizer interface {
	GenerateText(ctx context.Context, system, user string) (string, error)
}

// summarySystem 은 거칠게 추출된 대화를 지식 노트로 정리하는 지시문이다.
const summarySystem = `다음은 ChatGPT 대화를 거칠게 추출한 텍스트다(잡음·중복·UI 문구가 섞일 수 있다).
한 개발자의 개인 지식저장소에 넣을 간결한 한국어 요약 노트로 정리해라.
- 인사·잡담·중복·UI 문구는 버리고 핵심 개념과 결론만 남겨라.
- 마크다운으로 작성하고, ## 핵심 / ## 상세 같은 섹션을 자유롭게 구성해라.
- frontmatter(---)는 넣지 마라. 본문만 출력해라.`

// Service 는 공유 대화 추출→요약, 요약 저장을 묶는다.
type Service struct {
	sum      summarizer
	repoPath string
}

// NewService 는 Service 를 생성한다.
func NewService(sum summarizer, repoPath string) Service {
	return Service{sum: sum, repoPath: repoPath}
}

// Summarize 는 공유 링크 대화를 추출해 요약한다(제목 + 요약 본문). 저장하지 않는다.
func (s Service) Summarize(ctx context.Context, url string) (string, string, error) {
	conv, err := FetchConversation(ctx, url)
	if err != nil {
		return "", "", err
	}
	summary, err := s.sum.GenerateText(ctx, summarySystem, strings.Join(conv.Messages, "\n"))
	if err != nil {
		return "", "", err
	}
	return conv.Title, strings.TrimSpace(summary), nil
}

// SaveSource 는 (수정 반영된) 요약을 sources/ 에 저장한다.
func (s Service) SaveSource(_ context.Context, title, url, content string) (string, error) {
	today := time.Now().Format("2006-01-02")
	return WriteSource(s.repoPath, today, title, url, content)
}
