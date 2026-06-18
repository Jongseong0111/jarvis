package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

const maxTurns = 6

// DefaultSystemPrompt 는 에이전트의 기본 지시문이다.
const DefaultSystemPrompt = `너는 사용자의 개인 비서 '자비스'다. 친근하고 간결한 한국어로 대화한다.

핵심 규칙:
- 인사나 잡담에는 도구 없이 자연스럽게 답한다.
- 조회/등록 요청에는 **반드시 해당 도구를 호출**한다. "추가할게요", "알려드릴게요" 라고 말로만 답하고 도구를 안 부르면 안 된다. 실제 작업은 도구 호출로만 일어난다.
- 정보가 충분하면(예: 장소 이름 + 구역) 곧장 도구를 호출한다. 부족할 때만 되묻는다.
- 새 장소를 없던 구역에 추가하려면 add_location 의 zone 에 그 구역 이름을 그대로 넣으면 된다(구역은 자동 생성됨). "구역 먼저 만들고 장소 만들기"는 add_location 한 번이면 된다.
- **물건을 추가/수정할 때는 항상 category(대분류)를 지정한다.** 예: 세제·수세미=청소용품, 보험·계약서=서류, 충전기=전자기기, 기저귀=육아용품. 기존 카테고리가 궁금하면 list_categories 로 확인하고 가능하면 거기서 고른다. 마땅한 게 없으면 적절한 새 이름을 지정하면 자동 생성된다.
- 등록(쓰기) 도구를 호출하면 사용자에게 승인 버튼이 가는 변경안이 만들어진다. 그러니 등록 요청이면 망설이지 말고 도구를 호출한다.
- 도구 결과를 바탕으로 짧고 명확하게 답한다.`

// generator 는 도구와 함께 생성하는 능력이다(테스트에서 fake 주입).
type generator interface {
	GenerateWithTools(ctx context.Context, contents []*genai.Content, tools []*genai.Tool, system string) (*genai.GenerateContentResponse, error)
}

// VisionExtractor 는 이미지에서 물건 이름 목록을 뽑는 능력이다(테스트에서 fake 주입).
type VisionExtractor interface {
	ExtractItems(ctx context.Context, images []domain.Image) ([]string, error)
}

// Agent 는 도구를 가진 LLM 에이전트다. domain.MessageRouter 를 구현한다.
type Agent struct {
	gen    generator
	vision VisionExtractor
	tools  map[string]Tool
	decls  []*genai.Tool
	system string
	mem    *memory
}

// New 는 Agent 를 생성한다. vision 은 nil 가능(이미지 입력 미사용 시).
func New(gen generator, vision VisionExtractor, tools []Tool, system string) Agent {
	if system == "" {
		system = DefaultSystemPrompt
	}
	return Agent{gen: gen, vision: vision, tools: toolMap(tools), decls: toolDecls(tools), system: system, mem: newMemory()}
}

// Route 는 메시지를 에이전트 루프로 처리한다(잡담→텍스트, 작업→도구/변경안).
func (a Agent) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	if len(in.Images) > 0 && a.vision != nil {
		names, err := a.vision.ExtractItems(ctx, in.Images)
		switch {
		case err != nil:
			log.FromContext(ctx).Error("비전 추출 실패", "error", err) // best-effort: 텍스트로 진행
		case len(names) == 0:
			if strings.TrimSpace(in.Text) == "" {
				return domain.Reply{ChannelID: in.ChannelID, Text: "사진에서 물건을 못 찾았어. 뭐가 있는지 말로 알려줄래?"}, nil
			}
		default:
			in.Text = "[사진에서 인식한 물건: " + strings.Join(names, ", ") + "] " + in.Text
		}
	}

	contents := append(a.mem.get(in.ChannelID), genai.Text(in.Text)...)
	lastResult := "" // 마지막 읽기 도구 결과(모델이 빈 응답일 때 fallback)

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.gen.GenerateWithTools(ctx, contents, a.decls, a.system)
		if err != nil {
			return domain.Reply{}, fmt.Errorf("agent 생성 실패: %w", err)
		}

		fc := firstFunctionCall(resp)
		if fc == nil {
			text := finalText(resp, lastResult)
			a.mem.add(in.ChannelID, in.Text, text)
			return domain.Reply{ChannelID: in.ChannelID, Text: text}, nil
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
				// resolve 실패 등 → 모델에 알려주고 되묻게 한다. 모델이 빈 응답이면 이 사유를 보여준다.
				lastResult = err.Error()
				contents = append(contents, modelContent, funcResp(fc.Name, map[string]any{"error": err.Error()}))
				continue
			}
			// 메모리엔 변경안 원문을 넣지 않는다(모델이 그 형식을 텍스트로 흉내 내는 것 방지).
			a.mem.add(in.ChannelID, in.Text, "(승인 버튼이 있는 변경안을 사용자에게 보냈다)")
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
