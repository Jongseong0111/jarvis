package router

import (
	"context"
	"errors"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

// fakeClassifier 는 고정된 intent/에러를 반환하는 테스트용 Classifier 다.
type fakeClassifier struct {
	intent domain.Intent
	err    error
}

func (f fakeClassifier) Classify(_ context.Context, _ string) (domain.Intent, error) {
	return f.intent, f.err
}

// fakeWorker 는 자신의 라벨을 응답에 담는 테스트용 Worker 다.
type fakeWorker struct {
	label string
}

func (w fakeWorker) Handle(_ context.Context, intent domain.Intent, in domain.IncomingMessage) (domain.Reply, error) {
	return domain.Reply{ChannelID: in.ChannelID, Text: w.label + ":" + string(intent)}, nil
}

func TestRouter_Route(t *testing.T) {
	t.Parallel()

	home := fakeWorker{label: "home"}
	knowledge := fakeWorker{label: "knowledge"}
	system := fakeWorker{label: "system"}
	workers := map[string]domain.Worker{"home": home, "knowledge": knowledge}

	tests := []struct {
		name        string
		intent      domain.Intent
		classifyErr error
		wantText    string
		wantErr     bool
	}{
		{name: "home.* → home worker", intent: domain.IntentHomeAdd, wantText: "home:home.add"},
		{name: "knowledge.* → knowledge worker", intent: domain.IntentKnowledgeUpdate, wantText: "knowledge:knowledge.update"},
		{name: "system.* → fallback worker", intent: domain.IntentSystemHelp, wantText: "system:system.help"},
		{name: "unknown → fallback worker", intent: domain.IntentUnknown, wantText: "system:system.unknown"},
		{name: "분류 에러 → 에러 전파", classifyErr: errors.New("boom"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRouter(fakeClassifier{intent: tt.intent, err: tt.classifyErr}, workers, system)
			reply, err := r.Route(context.Background(), domain.IncomingMessage{ChannelID: "C1", Text: "x"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Route() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if reply.Text != tt.wantText {
				t.Fatalf("reply.Text = %q, want %q", reply.Text, tt.wantText)
			}
			if reply.ChannelID != "C1" {
				t.Fatalf("reply.ChannelID = %q, want C1", reply.ChannelID)
			}
		})
	}
}
