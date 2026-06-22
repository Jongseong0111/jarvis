package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func TestCalendarApplier_Delete(t *testing.T) {
	t.Parallel()
	port := &fakeCalPort{}
	ap := NewCalendarApplier(port)
	reply, err := ap.Apply(context.Background(), domain.ChangeProposal{
		Op:     "delete_event",
		Fields: map[string]string{"event_id": "e9", "summary": "치과 예약"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if port.deletedID != "e9" {
		t.Fatalf("삭제된 ID = %q, want e9", port.deletedID)
	}
	if !strings.Contains(reply.Text, "치과 예약") {
		t.Fatalf("응답에 제목 없음: %q", reply.Text)
	}
}

func TestCalendarApplier_WrongOp(t *testing.T) {
	t.Parallel()
	ap := NewCalendarApplier(&fakeCalPort{})
	if _, err := ap.Apply(context.Background(), domain.ChangeProposal{Op: "delete_todo"}); err == nil {
		t.Fatal("지원하지 않는 op 인데 에러가 없음")
	}
}
