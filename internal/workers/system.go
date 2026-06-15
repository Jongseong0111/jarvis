package workers

import (
	"context"

	"github.com/Jongseong0111/jarvis/domain"
)

const helpText = `나는 네 개인 비서야. 지금은 이런 걸 할 수 있어:

- 집 정리: "아기방 서랍에 손수건 10개 넣었어", "건전지 어디 뒀지?"
- 지식 저장소: "TLS 개념 정리해서 저장해줘", "ECS Role 차이 정리해줘"

(아직 실제 연동은 준비 중이라, 지금은 무슨 작업인지 인식하는 단계야.)`

const unknownText = "무슨 작업인지 잘 모르겠어. 집 정리(물건 위치)나 지식 저장소(개념 정리) 중 뭐에 관한 거야?"

// System 은 system.* intent(도움말/알 수 없음)를 처리하며, 매핑되지 않은 intent 의 fallback 이다.
type System struct{}

// NewSystem 은 System Worker 를 생성한다.
func NewSystem() System { return System{} }

// Handle 은 system intent 를 도움말 또는 되묻기 응답으로 변환한다.
func (System) Handle(_ context.Context, intent domain.Intent, in domain.IncomingMessage) (domain.Reply, error) {
	text := unknownText
	if intent == domain.IntentSystemHelp {
		text = helpText
	}
	return domain.Reply{ChannelID: in.ChannelID, Text: text}, nil
}
