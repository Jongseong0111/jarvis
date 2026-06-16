package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

const maxTurns = 6

// DefaultSystemPrompt 는 에이전트의 기본 지시문이다.
const DefaultSystemPrompt = `너는 사용자의 개인 비서 '자비스'다. 친근하고 간결한 한국어로 대화한다.

- 인사나 잡담에는 도구 없이 자연스럽게 답한다.
- 집 정리(물건/장소/구역/카테고리 조회·등록)는 제공된 도구를 사용한다.
- 물건이나 장소를 등록할 때 location/zone 은 가능하면 기존 것에서 고른다. 애매하면 사용자에게 되묻는다.
- 도구 실행 결과를 바탕으로 짧고 명확하게 답한다. 길게 설명하지 않는다.
- 등록(쓰기)은 사용자 승인 후에만 반영되니, 도구를 호출하면 변경안이 만들어진다.`

// generator 는 도구와 함께 생성하는 능력이다(테스트에서 fake 주입).
type generator interface {
	GenerateWithTools(ctx context.Context, contents []*genai.Content, tools []*genai.Tool, system string) (*genai.GenerateContentResponse, error)
}

// Agent 는 도구를 가진 LLM 에이전트다. domain.MessageRouter 를 구현한다.
type Agent struct {
	gen    generator
	tools  map[string]Tool
	decls  []*genai.Tool
	system string
}

// New 는 Agent 를 생성한다.
func New(gen generator, tools []Tool, system string) Agent {
	if system == "" {
		system = DefaultSystemPrompt
	}
	return Agent{gen: gen, tools: toolMap(tools), decls: toolDecls(tools), system: system}
}

// Route 는 메시지를 에이전트 루프로 처리한다(잡담→텍스트, 작업→도구/변경안).
func (a Agent) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	contents := genai.Text(in.Text)
	lastResult := "" // 마지막 읽기 도구 결과(모델이 빈 응답일 때 fallback)

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.gen.GenerateWithTools(ctx, contents, a.decls, a.system)
		if err != nil {
			return domain.Reply{}, fmt.Errorf("agent 생성 실패: %w", err)
		}

		fc := firstFunctionCall(resp)
		if fc == nil {
			return domain.Reply{ChannelID: in.ChannelID, Text: finalText(resp, lastResult)}, nil
		}

		modelContent := resp.Candidates[0].Content
		tool, ok := a.tools[fc.Name]
		if !ok {
			contents = append(contents, modelContent, funcResp(fc.Name, map[string]any{"error": "알 수 없는 도구"}))
			continue
		}

		if tool.Write {
			p, err := tool.Propose(ctx, fc.Args)
			if err != nil {
				// resolve 실패 등 → 모델에 알려주고 되묻게 한다.
				contents = append(contents, modelContent, funcResp(fc.Name, map[string]any{"error": err.Error()}))
				continue
			}
			return proposalReply(in.ChannelID, p), nil
		}

		result, err := tool.Run(ctx, fc.Args)
		if err != nil {
			contents = append(contents, modelContent, funcResp(fc.Name, map[string]any{"error": err.Error()}))
			continue
		}
		lastResult = result
		contents = append(contents, modelContent, funcResp(fc.Name, map[string]any{"result": result}))
	}

	return domain.Reply{ChannelID: in.ChannelID, Text: "조금 복잡한 요청이야. 더 구체적으로 말해줄래?"}, nil
}

// proposalReply 는 변경안을 요약 + 승인/취소 버튼이 달린 Reply 로 만든다.
func proposalReply(channelID string, p domain.ChangeProposal) domain.Reply {
	return domain.Reply{
		ChannelID: channelID,
		Text:      p.Summary + "\n\n적용할까?",
		Buttons: []domain.Button{
			{Text: "승인", Action: "approve", Value: p.Encode(), Style: "primary"},
			{Text: "취소", Action: "cancel"},
		},
	}
}

func firstFunctionCall(resp *genai.GenerateContentResponse) *genai.FunctionCall {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil
	}
	for _, p := range resp.Candidates[0].Content.Parts {
		if p.FunctionCall != nil {
			return p.FunctionCall
		}
	}
	return nil
}

// finalText 는 모델의 최종 텍스트를 반환한다. 비어있으면 마지막 도구 결과를, 그것도 없으면 안내문을.
func finalText(resp *genai.GenerateContentResponse, lastResult string) string {
	if resp != nil {
		if t := strings.TrimSpace(resp.Text()); t != "" {
			return t
		}
	}
	if lastResult != "" {
		return lastResult
	}
	return "음, 뭐라고 답해야 할지 모르겠어."
}

func funcResp(name string, data map[string]any) *genai.Content {
	return &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{Name: name, Response: data}}},
	}
}
