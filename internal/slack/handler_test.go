package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func Test_buildEcho(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       domain.IncomingMessage
		wantText string
		wantOK   bool
	}{
		{
			name:     "멘션 토큰 제거 후 echo",
			in:       domain.IncomingMessage{ChannelID: "C1", Text: "<@U123> 안녕"},
			wantText: "안녕",
			wantOK:   true,
		},
		{
			name:     "DM 평문 echo",
			in:       domain.IncomingMessage{ChannelID: "D1", Text: "건전지 어디 뒀지?"},
			wantText: "건전지 어디 뒀지?",
			wantOK:   true,
		},
		{
			name:   "멘션만 있고 본문 없음 → 응답 안 함",
			in:     domain.IncomingMessage{ChannelID: "C1", Text: "<@U123>   "},
			wantOK: false,
		},
		{
			name:     "파이프 포맷 멘션 제거 후 echo",
			in:       domain.IncomingMessage{ChannelID: "C1", Text: "<@U123|길동> 안녕"},
			wantText: "안녕",
			wantOK:   true,
		},
		{
			name:     "복수 멘션 제거 후 echo",
			in:       domain.IncomingMessage{ChannelID: "C1", Text: "<@U1> <@U2> 회의"},
			wantText: "회의",
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reply, ok := buildEcho(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("buildEcho ok = %v, want %v", ok, tt.wantOK)
			}
			if ok {
				if reply.Text != tt.wantText {
					t.Fatalf("buildEcho text = %q, want %q", reply.Text, tt.wantText)
				}
				if reply.ChannelID != tt.in.ChannelID {
					t.Fatalf("buildEcho channel = %q, want %q", reply.ChannelID, tt.in.ChannelID)
				}
			}
		})
	}
}

// fakeSender 는 테스트용 MessageSender 구현이다.
type fakeSender struct {
	sent []domain.Reply
	err  error
}

func (f *fakeSender) Send(_ context.Context, reply domain.Reply) error {
	f.sent = append(f.sent, reply)
	return f.err
}

func TestHandler_Handle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		in          domain.IncomingMessage
		senderErr   error
		wantSent    int
		wantText    string
		wantErr     bool
		wantErrWrap error
	}{
		{
			name:     "본문 있는 메시지 → Send 1회 호출",
			in:       domain.IncomingMessage{ChannelID: "C1", UserID: "U1", Text: "<@U123> 안녕"},
			wantSent: 1,
			wantText: "안녕",
			wantErr:  false,
		},
		{
			name:     "빈 본문 → Send 호출 안 됨, 에러 nil",
			in:       domain.IncomingMessage{ChannelID: "C1", UserID: "U1", Text: "<@U123>  "},
			wantSent: 0,
			wantErr:  false,
		},
		{
			name:        "sender 에러 → 래핑된 에러 반환",
			in:          domain.IncomingMessage{ChannelID: "C1", UserID: "U1", Text: "안녕"},
			senderErr:   errors.New("network error"),
			wantSent:    1,
			wantErr:     true,
			wantErrWrap: errors.New("network error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fs := &fakeSender{err: tt.senderErr}
			h := NewHandler(fs)
			err := h.Handle(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Handle() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.senderErr != nil {
				if !errors.Is(err, tt.senderErr) {
					t.Fatalf("Handle() error = %v, want wrapped %v", err, tt.senderErr)
				}
			}
			if len(fs.sent) != tt.wantSent {
				t.Fatalf("Send 호출 횟수 = %d, want %d", len(fs.sent), tt.wantSent)
			}
			if tt.wantSent > 0 && tt.wantText != "" {
				if fs.sent[0].Text != tt.wantText {
					t.Fatalf("reply.Text = %q, want %q", fs.sent[0].Text, tt.wantText)
				}
			}
		})
	}
}
