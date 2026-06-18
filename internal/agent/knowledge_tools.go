package agent

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// KnowledgePort 는 지식 도구가 필요로 하는 작업이다(테스트에서 fake 주입).
type KnowledgePort interface {
	Summarize(ctx context.Context, url string) (title string, summary string, err error)
	SaveSource(ctx context.Context, title, url, content string) (path string, err error)
}

type knowledgeTools struct {
	port KnowledgePort
}

// KnowledgeTools 는 ChatGPT 공유링크 요약/저장 도구 목록을 만든다(둘 다 읽기형).
func KnowledgeTools(port KnowledgePort) []Tool {
	k := knowledgeTools{port: port}
	return []Tool{k.summarizeShare(), k.saveSource()}
}

func (k knowledgeTools) summarizeShare() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "summarize_chatgpt_share",
			Description: "ChatGPT 공유 링크(chatgpt.com/share/...)의 대화를 추출해 요약한다. 저장은 하지 않는다(보여주기만).",
			Parameters: objSchema(map[string]*genai.Schema{
				"url": strSchema("ChatGPT 공유 링크 URL"),
			}, "url"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			title, summary, err := k.port.Summarize(ctx, strArg(args, "url"))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("제목: %s\n\n%s", title, summary), nil
		},
	}
}

func (k knowledgeTools) saveSource() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "save_kb_source",
			Description: "사용자가 요약을 확정(예: '저장해')하면 지식저장소 sources/ 에 저장한다. content 에는 현재 대화에서 보여준 (수정 반영된) 요약 본문 전체를 넣어라.",
			Parameters: objSchema(map[string]*genai.Schema{
				"title":   strSchema("문서 제목"),
				"content": strSchema("저장할 요약 본문 전체(마크다운, 수정 반영된 최종본)"),
				"url":     strSchema("출처 ChatGPT 공유 링크(있으면)"),
			}, "title", "content"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			path, err := k.port.SaveSource(ctx, strArg(args, "title"), strArg(args, "url"), strArg(args, "content"))
			if err != nil {
				return "", err
			}
			return "저장했어: " + path, nil
		},
	}
}
