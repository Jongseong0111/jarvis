// Package domain 은 채널 독립적인 인터페이스/DTO 를 정의한다(구현 없음).
package domain

import "context"

// IncomingMessage 는 채널에서 수신한 사용자 메시지다.
type IncomingMessage struct {
	ChannelID string
	UserID    string
	Text      string
}

// Reply 는 채널로 보낼 응답이다.
type Reply struct {
	ChannelID string
	Text      string
}

// MessageSender 는 채널로 메시지를 전송하는 능력이다.
type MessageSender interface {
	Send(ctx context.Context, reply Reply) error
}
