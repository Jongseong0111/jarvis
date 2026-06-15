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
	Buttons   []Button // 비면 일반 텍스트 응답 (Phase 1/2 와 호환)
}

// Button 은 채널 독립적 액션 버튼이다. 채널 어댑터가 렌더링한다.
type Button struct {
	Text   string // 표시 라벨 ("승인"/"취소")
	Action string // "approve" / "cancel"
	Value  string // 액션에 필요한 직렬화 데이터 (ChangeProposal JSON)
	Style  string // "primary"/"danger"/"" (선택)
}

// MessageSender 는 채널로 메시지를 전송하는 능력이다.
type MessageSender interface {
	Send(ctx context.Context, reply Reply) error
}
