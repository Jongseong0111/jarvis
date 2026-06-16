package slack

import (
	"context"
	"sync"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
	slackgo "github.com/slack-go/slack"
)

const applyErrText = "처리 중 문제가 생겼어. 잠시 후 다시 시도해줘."

// InteractionHandler 는 버튼 클릭(승인/취소)을 처리한다.
type InteractionHandler struct {
	applier domain.ProposalApplier
	sender  domain.MessageSender

	mu   sync.Mutex
	seen map[string]bool // 이미 처리한 메시지 ts(더블클릭 방지)
}

// NewInteractionHandler 는 InteractionHandler 를 생성한다.
func NewInteractionHandler(applier domain.ProposalApplier, sender domain.MessageSender) *InteractionHandler {
	return &InteractionHandler{applier: applier, sender: sender, seen: map[string]bool{}}
}

// Handle 은 버튼 콜백에서 액션/값을 추출해 처리하고 결과를 채널로 전송한다.
// 같은 메시지의 버튼을 두 번 누르면(더블클릭) 두 번째는 무시한다.
func (h *InteractionHandler) Handle(ctx context.Context, callback slackgo.InteractionCallback) error {
	actions := callback.ActionCallback.BlockActions
	if len(actions) == 0 {
		return nil
	}
	if ts := callback.Container.MessageTs; ts != "" && h.alreadyHandled(ts) {
		log.FromContext(ctx).Info("중복 버튼 클릭 무시", "ts", ts)
		return nil
	}
	a := actions[0]
	reply, ok := h.resolve(ctx, callback.Channel.ID, a.ActionID, a.Value)
	if !ok {
		return nil
	}
	return h.sender.Send(ctx, reply)
}

// alreadyHandled 는 ts 를 처음 보면 기록하고 false, 이미 본 ts 면 true 를 반환한다.
func (h *InteractionHandler) alreadyHandled(ts string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.seen[ts] {
		return true
	}
	h.seen[ts] = true
	return false
}

// resolve 는 액션을 처리해 응답을 만든다. 처리할 게 없으면 ok=false. (SDK 비의존)
func (h *InteractionHandler) resolve(ctx context.Context, channelID, actionID, value string) (domain.Reply, bool) {
	reply := domain.Reply{ChannelID: channelID}
	switch actionID {
	case "approve":
		p, err := domain.DecodeProposal(value)
		if err != nil {
			log.FromContext(ctx).Error("변경안 디코드 실패", "error", err)
			reply.Text = applyErrText
			return reply, true
		}
		applied, err := h.applier.Apply(ctx, p)
		if err != nil {
			log.FromContext(ctx).Error("변경안 적용 실패", "error", err)
			reply.Text = applyErrText
			return reply, true
		}
		reply.Text = applied.Text
		return reply, true
	case "cancel":
		reply.Text = "취소했어."
		return reply, true
	default:
		return reply, false
	}
}
