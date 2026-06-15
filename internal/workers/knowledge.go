package workers

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
)

// Knowledge 는 지식 저장소(knowledge.*) intent 를 처리한다. Phase 4 에서 Claude Code 연동으로 채운다.
type Knowledge struct{}

// NewKnowledge 는 Knowledge Worker 를 생성한다.
func NewKnowledge() Knowledge { return Knowledge{} }

// Handle 은 인식된 knowledge intent 를 안내 응답으로 변환한다. (스텁)
func (Knowledge) Handle(_ context.Context, intent domain.Intent, in domain.IncomingMessage) (domain.Reply, error) {
	text := fmt.Sprintf("지식 저장소 작업으로 인식했어 (%s). 아직 Claude Code 연동은 준비 중이야.", intent)
	return domain.Reply{ChannelID: in.ChannelID, Text: text}, nil
}
