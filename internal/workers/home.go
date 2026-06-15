// Package workers 는 intent 별 작업을 수행하는 Worker 들을 구현한다.
// Phase 2 에서는 실제 외부 연동 없이 인식 결과만 응답하는 스텁이다.
package workers

import (
	"context"
	"fmt"

	"github.com/Jongseong0111/jarvis/domain"
)

// Home 은 집 정리(home.*) intent 를 처리한다. Phase 3 에서 Notion 연동으로 채운다.
type Home struct{}

// NewHome 은 Home Worker 를 생성한다.
func NewHome() Home { return Home{} }

// Handle 은 인식된 home intent 를 안내 응답으로 변환한다. (스텁)
func (Home) Handle(_ context.Context, intent domain.Intent, in domain.IncomingMessage) (domain.Reply, error) {
	text := fmt.Sprintf("집 정리 작업으로 인식했어 (%s). 아직 Notion 연동은 준비 중이야.", intent)
	return domain.Reply{ChannelID: in.ChannelID, Text: text}, nil
}
