// Package slack 은 Slack 채널 어댑터(연결/이벤트 처리/전송)를 구현한다.
package slack

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// mentionPattern 은 Slack 멘션 토큰(<@U123> 및 <@U123|name> 형태)을 매칭한다.
var mentionPattern = regexp.MustCompile(`<@[A-Z0-9]+(?:\|[^>]*)?>`)

// errorReplyText 는 라우팅 실패 시 사용자에게 보내는 짧은 안내다.
const errorReplyText = "처리 중 문제가 생겼어. 잠시 후 다시 시도해줘."

// Handler 는 수신 메시지를 라우터에 위임해 처리한다.
type Handler struct {
	router domain.MessageRouter
	sender domain.MessageSender
}

// NewHandler 는 Handler 를 생성한다.
func NewHandler(router domain.MessageRouter, sender domain.MessageSender) Handler {
	return Handler{router: router, sender: sender}
}

// Handle 은 멘션 토큰을 제거한 뒤 라우터로 응답을 만들어 전송한다.
// 라우팅 실패는 로그로 남기고 사용자에겐 짧은 안내를 보낸다.
func (h Handler) Handle(ctx context.Context, in domain.IncomingMessage) error {
	in.Text = cleanText(in.Text)
	if in.Text == "" {
		return nil
	}

	reply, err := h.router.Route(ctx, in)
	if err != nil {
		log.FromContext(ctx).Error("라우팅 실패", "error", err)
		reply = domain.Reply{ChannelID: in.ChannelID, Text: errorReplyText}
	}

	log.FromContext(ctx).Info("응답", "channel", reply.ChannelID, "text", reply.Text)
	if err := h.sender.Send(ctx, reply); err != nil {
		return fmt.Errorf("응답 전송 실패: %w", err)
	}
	return nil
}

// cleanText 는 Slack 멘션 토큰을 제거하고 공백을 정리한다. (Slack 전용)
func cleanText(s string) string {
	return strings.TrimSpace(mentionPattern.ReplaceAllString(s, ""))
}
