package agent

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
)

// calendarApplier 는 delete_event 변경안을 캘린더에 반영한다.
type calendarApplier struct{ port CalendarPort }

// NewCalendarApplier 는 일정 삭제 승인 처리기를 만든다.
func NewCalendarApplier(port CalendarPort) domain.ProposalApplier {
	return calendarApplier{port: port}
}

func (a calendarApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if p.Op != "delete_event" {
		return domain.Reply{}, fmt.Errorf("calendarApplier: 지원하지 않는 op %q", p.Op)
	}
	if err := a.port.DeleteEvent(ctx, p.Fields["event_id"]); err != nil {
		return domain.Reply{}, err
	}
	return domain.Reply{Text: "🗑️ '" + p.Fields["summary"] + "' 일정을 삭제했습니다."}, nil
}
