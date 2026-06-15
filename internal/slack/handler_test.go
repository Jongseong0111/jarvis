package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

// fakeSender 는 테스트용 MessageSender 구현이다.
type fakeSender struct {
	sent []domain.Reply
	err  error
}

func (f *fakeSender) Send(_ context.Context, reply domain.Reply) error {
	f.sent = append(f.sent, reply)
	return f.err
}

// fakeRouter 는 받은 메시지를 기록하고 고정 응답/에러를 반환하는 테스트용 MessageRouter 다.
type fakeRouter struct {
	got   domain.IncomingMessage
	reply domain.Reply
	err   error
}

func (f *fakeRouter) Route(_ context.Context, in domain.IncomingMessage) (domain.Reply, error) {
	f.got = in
	return f.reply, f.err
}

func TestHandler_Handle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		in         domain.IncomingMessage
		routerRep  domain.Reply
		routerErr  error
		senderErr  error
		wantRouted bool   // 라우터가 호출되었는지
		wantText   string // 라우터에 전달된(정리된) 텍스트
		wantSent   int
		wantReply  string // Send 된 응답 텍스트
		wantErr    bool
	}{
		{
			name:       "멘션 제거 후 라우터 위임 → 응답 전송",
			in:         domain.IncomingMessage{ChannelID: "C1", Text: "<@U123> 건전지 어디 뒀지?"},
			routerRep:  domain.Reply{ChannelID: "C1", Text: "home:home.search"},
			wantRouted: true,
			wantText:   "건전지 어디 뒀지?",
			wantSent:   1,
			wantReply:  "home:home.search",
		},
		{
			name:       "빈 본문 → 라우터 미호출, Send 안 함",
			in:         domain.IncomingMessage{ChannelID: "C1", Text: "<@U123>   "},
			wantRouted: false,
			wantSent:   0,
		},
		{
			name:       "라우터 에러 → 에러 안내 응답 전송",
			in:         domain.IncomingMessage{ChannelID: "C1", Text: "안녕"},
			routerErr:  errors.New("classify failed"),
			wantRouted: true,
			wantText:   "안녕",
			wantSent:   1,
			wantReply:  errorReplyText,
		},
		{
			name:       "sender 에러 → 래핑된 에러 반환",
			in:         domain.IncomingMessage{ChannelID: "C1", Text: "안녕"},
			routerRep:  domain.Reply{ChannelID: "C1", Text: "ok"},
			senderErr:  errors.New("network error"),
			wantRouted: true,
			wantText:   "안녕",
			wantSent:   1,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fr := &fakeRouter{reply: tt.routerRep, err: tt.routerErr}
			fs := &fakeSender{err: tt.senderErr}
			h := NewHandler(fr, fs)

			err := h.Handle(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Handle() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.senderErr != nil && !errors.Is(err, tt.senderErr) {
				t.Fatalf("Handle() error = %v, want wrapped %v", err, tt.senderErr)
			}
			if tt.wantRouted && fr.got.Text != tt.wantText {
				t.Fatalf("라우터 전달 텍스트 = %q, want %q", fr.got.Text, tt.wantText)
			}
			if !tt.wantRouted && fr.got.Text != "" {
				t.Fatalf("라우터가 호출되지 않아야 하는데 호출됨: %q", fr.got.Text)
			}
			if len(fs.sent) != tt.wantSent {
				t.Fatalf("Send 호출 횟수 = %d, want %d", len(fs.sent), tt.wantSent)
			}
			if tt.wantReply != "" && fs.sent[0].Text != tt.wantReply {
				t.Fatalf("Send 응답 = %q, want %q", fs.sent[0].Text, tt.wantReply)
			}
		})
	}
}
