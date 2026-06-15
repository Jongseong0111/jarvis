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


// Handler 는 수신 메시지를 echo 응답으로 처리한다.
type Handler struct {
	sender domain.MessageSender
}

// NewHandler 는 Handler 를 생성한다.
func NewHandler(sender domain.MessageSender) Handler {
	return Handler{sender: sender}
}

// Handle 은 수신 메시지를 echo 로 변환해 전송한다.
func (h Handler) Handle(ctx context.Context, in domain.IncomingMessage) error {
	reply, ok := buildEcho(in)
	if !ok {
		return nil
	}
	log.FromContext(ctx).Info("echo 응답", "channel", reply.ChannelID, "text", reply.Text)
	if err := h.sender.Send(ctx, reply); err != nil {
		return fmt.Errorf("echo 전송 실패: %w", err)
	}
	return nil
}

// buildEcho 는 수신 메시지로부터 echo 응답을 계산한다. SDK/네트워크 비의존.
func buildEcho(in domain.IncomingMessage) (domain.Reply, bool) {
	text := strings.TrimSpace(mentionPattern.ReplaceAllString(in.Text, ""))
	if text == "" {
		return domain.Reply{}, false
	}
	return domain.Reply{ChannelID: in.ChannelID, Text: text}, true
}
