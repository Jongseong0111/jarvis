package agent

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

// ReviewRouter 는 채널이 리뷰 모드일 때 claude 세션으로, 아닐 때 기본 에이전트로 라우팅한다.
// domain.MessageRouter 를 구현한다.
type ReviewRouter struct {
	base     domain.MessageRouter
	registry *ReviewSessionRegistry
	runner   claudecode.Runner
	sender   domain.MessageSender
}

// NewReviewRouter 는 ReviewRouter 를 만든다.
func NewReviewRouter(base domain.MessageRouter, registry *ReviewSessionRegistry, runner claudecode.Runner, sender domain.MessageSender) *ReviewRouter {
	return &ReviewRouter{base: base, registry: registry, runner: runner, sender: sender}
}

// Route 는 채널 상태에 따라 메시지를 라우팅한다.
func (r *ReviewRouter) Route(ctx context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	session, inReview := r.registry.Get(in.ChannelID)
	if !inReview {
		return r.base.Route(ctx, in)
	}

	if session.Busy {
		return domain.Reply{ChannelID: in.ChannelID, Text: "⏳ 아직 처리 중이에요. 잠시 후 말씀해주세요."}, nil
	}

	text := strings.TrimSpace(in.Text)

	if isCancelKeyword(text) {
		r.registry.Exit(in.ChannelID)
		return domain.Reply{
			ChannelID: in.ChannelID,
			Text:      "❌ 개념 정리를 취소했습니다. 브랜치(" + session.Branch + ")는 남아있어요.",
		}, nil
	}

	if isApproveKeyword(text) {
		return r.approve(in.ChannelID, session)
	}

	return r.curate(in.ChannelID, session, text)
}

// curate 는 큐레이션 메시지를 비동기로 처리하고 즉시 대기 안내를 반환한다.
func (r *ReviewRouter) curate(channelID string, session ReviewSession, text string) (domain.Reply, error) {
	r.registry.SetBusy(channelID, true)
	go func() {
		bgCtx := context.Background()
		result, err := r.runner.Resume(bgCtx, session.SessionID, text)
		r.registry.SetBusy(channelID, false)
		if err != nil {
			slog.Default().Error("curate resume 실패", "channel", channelID, "error", err)
			_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: "🚨 처리 중 문제가 생겼어요."})
			return
		}
		_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: result.Text})
	}()
	return domain.Reply{ChannelID: channelID, Text: "⏳ 처리 중입니다..."}, nil
}

// approve 는 승인 처리를 비동기로 수행하고 즉시 안내를 반환한다.
func (r *ReviewRouter) approve(channelID string, session ReviewSession) (domain.Reply, error) {
	r.registry.SetBusy(channelID, true)
	approvePrompt := "/kb-approve 한 뒤, 이 브랜치를 push 하고 gh 로 main 대상 PR 을 생성해줘."
	go func() {
		bgCtx := context.Background()
		result, err := r.runner.Resume(bgCtx, session.SessionID, approvePrompt)
		r.registry.Exit(channelID)
		if err != nil {
			slog.Default().Error("approve 실패", "channel", channelID, "error", err)
			_ = r.sender.Send(bgCtx, domain.Reply{
				ChannelID: channelID,
				Text:      "🚨 승인 처리 중 문제가 생겼어요. 브랜치(" + session.Branch + ")는 남아있어요.",
			})
			return
		}
		_ = r.sender.Send(bgCtx, domain.Reply{ChannelID: channelID, Text: result.Text})
	}()
	return domain.Reply{ChannelID: channelID, Text: "✅ 승인 처리 중입니다..."}, nil
}

var cancelKeywords = []string{"취소", "그만", "cancel", "stop"}
var approveKeywords = []string{"승인", "이대로", "approve"}

// isCancelKeyword 는 텍스트에 취소 키워드가 포함됐는지 확인한다.
func isCancelKeyword(s string) bool {
	for _, kw := range cancelKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// isApproveKeyword 는 텍스트에 승인 키워드가 포함됐는지 확인한다.
func isApproveKeyword(s string) bool {
	for _, kw := range approveKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
