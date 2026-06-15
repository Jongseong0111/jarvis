package workers

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func TestKnowledge_Handle(t *testing.T) {
	t.Parallel()
	reply, err := NewKnowledge().Handle(context.Background(), domain.IntentKnowledgeUpdate, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(reply.Text, string(domain.IntentKnowledgeUpdate)) {
		t.Fatalf("응답에 intent 표시가 없음: %q", reply.Text)
	}
}

func TestSystem_Handle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		intent   domain.Intent
		wantText string
	}{
		{name: "help → 도움말", intent: domain.IntentSystemHelp, wantText: helpText},
		{name: "unknown → 되묻기", intent: domain.IntentUnknown, wantText: unknownText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reply, err := NewSystem().Handle(context.Background(), tt.intent, domain.IncomingMessage{ChannelID: "C1"})
			if err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if reply.Text != tt.wantText {
				t.Fatalf("reply.Text = %q, want %q", reply.Text, tt.wantText)
			}
		})
	}
}
