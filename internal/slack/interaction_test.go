package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

type fakeApplier struct {
	got   domain.ChangeProposal
	reply domain.Reply
	err   error
}

func (f *fakeApplier) Apply(_ context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	f.got = p
	return f.reply, f.err
}

func TestInteraction_resolve(t *testing.T) {
	t.Parallel()
	validValue := domain.ChangeProposal{Op: "add_item", Summary: "체온계 추가", Fields: map[string]string{"name": "체온계", "location_id": "loc-1", "location_name": "아기 트롤리"}}.Encode()

	tests := []struct {
		name      string
		actionID  string
		value     string
		applyRep  domain.Reply
		applyErr  error
		wantSend  bool
		wantText  string
		wantApply bool
	}{
		{name: "approve → Apply 호출 후 결과 전송", actionID: "approve", value: validValue,
			applyRep: domain.Reply{Text: "✅ 추가했어"}, wantSend: true, wantText: "✅ 추가했어", wantApply: true},
		{name: "approve 잘못된 value → 에러 안내", actionID: "approve", value: "not-json",
			wantSend: true, wantText: applyErrText, wantApply: false},
		{name: "approve Apply 실패 → 에러 안내", actionID: "approve", value: validValue,
			applyErr: errors.New("notion down"), wantSend: true, wantText: applyErrText, wantApply: true},
		{name: "cancel → 취소 안내", actionID: "cancel", wantSend: true, wantText: "취소했어.", wantApply: false},
		{name: "알 수 없는 액션 → 전송 안 함", actionID: "noop", wantSend: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ap := &fakeApplier{reply: tt.applyRep, err: tt.applyErr}
			h := NewInteractionHandler(ap, &fakeSender{})
			reply, ok := h.resolve(context.Background(), "C1", tt.actionID, tt.value)
			if ok != tt.wantSend {
				t.Fatalf("send 여부 = %v, want %v", ok, tt.wantSend)
			}
			if !tt.wantSend {
				return
			}
			if reply.ChannelID != "C1" {
				t.Fatalf("ChannelID = %q, want C1", reply.ChannelID)
			}
			if reply.Text != tt.wantText {
				t.Fatalf("Text = %q, want %q", reply.Text, tt.wantText)
			}
			if tt.wantApply && ap.got.Fields["name"] != "체온계" {
				t.Fatalf("Apply 에 전달된 변경안 = %+v", ap.got)
			}
		})
	}
}
