package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

const maxTurns = 6

// agentCtxKey 는 에이전트 내부 컨텍스트 키 타입이다.
type agentCtxKey int

const channelIDKey agentCtxKey = 1

// WithChannelID 는 channelID 를 컨텍스트에 저장한다(ingest 도구 등에서 조회).
func WithChannelID(ctx context.Context, channelID string) context.Context {
	return context.WithValue(ctx, channelIDKey, channelID)
}

// channelIDFromCtx 는 컨텍스트에서 channelID 를 꺼낸다(없으면 "").
func channelIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(channelIDKey).(string)
	return v
}

// DefaultSystemPrompt 는 에이전트의 기본 지시문이다.
const DefaultSystemPrompt = `너는 사용자의 개인 비서 '자비스'다. 친근하면서도 정중한 **존댓말**(~합니다/~해요)로, 간결한 한국어로 대화한다.

핵심 규칙:
- 인사나 잡담에는 도구 없이 자연스럽게 답한다.
- 목록을 보여줄 땐 별표(*) 대신 가운뎃점(•)으로 항목을 나열하고, 어울리는 이모지를 적절히 곁들여 보기 좋게 답한다.
- 조회/등록 요청에는 **반드시 해당 도구를 호출**한다. "추가할게요", "알려드릴게요" 라고 말로만 답하고 도구를 안 부르면 안 된다. 실제 작업은 도구 호출로만 일어난다.
- 정보가 충분하면(예: 장소 이름 + 구역) 곧장 도구를 호출한다. 부족할 때만 되묻는다.
- 새 장소를 없던 구역에 추가하려면 add_location 의 zone 에 그 구역 이름을 그대로 넣으면 된다(구역은 자동 생성됨). "구역 먼저 만들고 장소 만들기"는 add_location 한 번이면 된다.
- **장소 이름(name)에 구역 이름을 넣지 마라.** 구역은 zone 으로 분리한다. 예: 구역 '로그 방'의 팬트리는 name="팬트리", zone="로그 방" (name="로그 방 팬트리" ❌). 같은 이름이 다른 구역에 있어도 zone 으로 구분되니 괜찮다.
- **물건을 추가/수정할 때는 항상 category(대분류)를 지정한다.** 예: 세제·수세미=청소용품, 보험·계약서=서류, 충전기=전자기기, 기저귀=육아용품. 기존 카테고리가 궁금하면 list_categories 로 확인하고 가능하면 거기서 고른다. 마땅한 게 없으면 적절한 새 이름을 지정하면 자동 생성된다.
- 등록(쓰기) 도구를 호출하면 사용자에게 승인 버튼이 가는 변경안이 만들어진다. 그러니 등록 요청이면 망설이지 말고 도구를 호출한다.
- 도구 결과를 바탕으로 짧고 명확하게 답한다.
- **도구의 함수 응답(JSON·{"...":...} 형태)을 사용자에게 그대로 출력하지 마라.** 그 내용을 읽고 자연스러운 한국어 문장·목록으로만 답한다.
- ChatGPT 공유 링크(chatgpt.com/share/... 또는 chat.openai.com/share/...)를 정리/요약 요청과 함께 받으면 summarize_chatgpt_share 도구를 그 URL 로 호출한다. 요약을 보여준 뒤 **바로 저장하지 마라.**
- 사용자가 요약 수정을 요청하면(예: "더 짧게", "이 부분 빼") 도구 없이 대화로 직접 고쳐 다시 보여줘라.
- 사용자가 "저장/저장해/이대로 저장" 등으로 확정하면 그때 save_kb_source 를 호출하되, content 에는 현재 보여준 (수정 반영된) 요약 본문 전체를 넣어라.
- 할일/투두 요청은 Todoist 도구를 써라. 추가/조회/완료/수정은 바로 실행하고 결과를 짧게 알려라.
- add_todo 로 추가할 때 '오늘·내일·모레·매주 월요일·오후 3시' 같은 **날짜·시간 표현은 content 가 아니라 due 에 분리**해 넣어라. content 엔 할일 내용만 남긴다. 예: "오늘 테스트할일 추가" → content="테스트할일", due="오늘".
- list_todos 기본은 오늘+밀린이다. 사용자가 '전체·모든·반복·매주·매일·스케줄' 등을 물으면 filter="all" 로 전체를 불러와 보여줘라(반복 작업은 due 에 "매주 토요일"처럼 표시된다). '관리함·아직 안 정한·마감/일정 없는' 할일을 물으면 filter="no date", '이번주'는 filter="7 days" 로 호출한다.
- 완료/수정/삭제는 먼저 query 로 할일을 찾는다. 모호하면 되묻는다.
- 삭제는 delete_todo 로 변경안을 만들어 승인 버튼을 거친다(바로 지우지 않는다).
- save_kb_source 로 소스를 저장한 직후, "저장했습니다! 이 내용을 개념 문서로 정리할까요? 🗂️" 라고 제안해라. 사용자가 "응/그래/해줘/해줘봐" 등 긍정 답변을 하면 start_concept_ingest 를 호출하되, source_path 에는 save_kb_source 가 반환한 경로(예: sources/conversation/xxx.md)를 넣어라.
- 사용자가 명시적으로 "개념 정리해줘", "지식 정리해줘" 등을 요청하면 start_concept_ingest 를 호출한다. source_path 가 언급되지 않으면 직전 대화에서 저장된 경로를 쓴다.
- 사용자가 "공부 주제 추천/다른 거/특정 도메인(운영체제 등) 주제"를 요청하면 suggest_study_topics 를 호출한다. 특정 도메인을 말하면 domain 인자에 넣고, 아니면 비운다.
- 사용자가 "비용/요금/얼마 썼어/LLM 비용" 등을 물으면 list_usage 를 호출해 답한다. 기간을 말하면 period 에 today/week/month 로 넣고, 아니면 비운다(오늘 기준).`

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
	gen     generator
	vision  VisionExtractor
	tools   map[string]Tool
	decls   []*genai.Tool
	system  string
	now     func() time.Time
	mem     *memory
	locHint func(ctx context.Context) string // 현재 장소 목록을 프롬프트에 주입(nil 가능)
}

// New 는 Agent 를 생성한다. vision 은 nil 가능(이미지 입력 미사용 시).
func New(gen generator, vision VisionExtractor, tools []Tool, system string) Agent {
	if system == "" {
		system = DefaultSystemPrompt
	}
	return Agent{gen: gen, vision: vision, tools: toolMap(tools), decls: toolDecls(tools), system: system, now: time.Now, mem: newMemory()}
}

// WithLocationsHint 는 매 메시지 시스템 프롬프트에 현재 장소 목록을 주입하는 함수를 단다(집정리 동명 장소 구분용).
func (a Agent) WithLocationsHint(fn func(ctx context.Context) string) Agent {
	a.locHint = fn
	return a
}

// CalendarSystemHint 는 캘린더 기능이 켜졌을 때 시스템 프롬프트에 덧붙이는 지시문이다.
const CalendarSystemHint = `
- 일정/캘린더 조회·추가·삭제·검색은 캘린더 도구(list_events/add_event/search_events/delete_event)를 쓴다. add_event 의 start/end 는 위 '현재 시각'을 기준으로 상대 표현을 RFC3339(예: 2026-06-29T15:00:00+09:00)나 종일이면 YYYY-MM-DD 로 변환해 넣는다.
- "급한 일/급한 거"를 물으면 캘린더(오늘~내일)와 할일(밀린·오늘·내일)을 모두 확인해 합쳐서 답한다.
- 일정 삭제는 delete_event 로 변경안을 만들어 승인 버튼을 거친다.`

// datedSystem 은 기본 시스템 프롬프트에 현재 시각(Asia/Seoul)을 덧붙인다.
// 서버가 장시간 떠 있어도 메시지마다 최신 날짜가 들어가도록 호출 시점에 계산한다.
func (a Agent) datedSystem() string {
	t := a.now().In(seoulLoc())
	return a.system + "\n\n[현재 시각: " + t.Format("2006-01-02 (Mon) 15:04") + " Asia/Seoul. 상대적 날짜·시간 표현은 이 시각 기준으로 해석한다.]"
}

// Route 는 메시지를 에이전트 루프로 처리한다(잡담→텍스트, 작업→도구/변경안).
func (a Agent) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	ctx = WithChannelID(ctx, in.ChannelID)
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
	system := a.datedSystem()
	if a.locHint != nil {
		if hint := a.locHint(ctx); hint != "" {
			system += "\n\n" + hint
		}
	}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := a.gen.GenerateWithTools(ctx, contents, a.decls, system)
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

	return domain.Reply{ChannelID: in.ChannelID, Text: "조금 복잡한 요청이에요. 더 구체적으로 말씀해 주시겠어요?"}, nil
}

// proposalReply 는 변경안을 요약 + 승인/취소 버튼이 달린 Reply 로 만든다.
func proposalReply(channelID string, p domain.ChangeProposal) domain.Reply {
	return domain.Reply{
		ChannelID: channelID,
		Text:      p.Summary + "\n\n적용할까요?",
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
	return "음, 어떻게 답해야 할지 모르겠어요."
}

func funcResp(name string, data map[string]any) *genai.Content {
	return &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{Name: name, Response: data}}},
	}
}
