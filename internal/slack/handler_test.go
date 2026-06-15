package slack

import (
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
