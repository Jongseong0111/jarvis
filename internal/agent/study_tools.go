package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/devdigest"
)

// StudyTopicGenerator 는 공부 주제를 생성하는 능력이다(테스트에서 fake 주입).
type StudyTopicGenerator interface {
	GenerateTopics(ctx context.Context, domain string) (devdigest.TopicResult, error)
}

type studyTools struct {
	gen StudyTopicGenerator
}

// StudyTools 는 공부 주제 추천 도구 목록을 만든다(읽기형).
func StudyTools(gen StudyTopicGenerator) []Tool {
	s := studyTools{gen: gen}
	return []Tool{s.suggestStudyTopics()}
}

func (s studyTools) suggestStudyTopics() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "suggest_study_topics",
			Description: "개발 공부 주제를 추천한다. 사용자가 '다른 공부 주제', '운영체제 주제', 'DB 다른 거' 등을 요청할 때 호출한다. domain 에 특정 도메인/주제를 넣으면 그 주제로, 비우면 임의 도메인으로 생성한다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"domain": strSchema("공부 도메인 또는 세부 주제. 예: 운영체제, 데이터베이스, 쿠버네티스. 미지정 시 임의 선택."),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			result, err := s.gen.GenerateTopics(ctx, strArg(args, "domain"))
			if err != nil {
				return "", err
			}
			return formatTopics(result), nil
		},
	}
}

// formatTopics 는 공부 주제를 Slack 텍스트로 만든다(아침 digest 공부주제 섹션과 동일 형식).
func formatTopics(r devdigest.TopicResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📚 *공부 주제*  _(도메인: %s)_\n", r.Domain))
	for _, topic := range r.Topics {
		sb.WriteString("• " + topic + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
