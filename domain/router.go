package domain

import (
	"context"
	"strings"
)

// Intent 는 수신 메시지가 어떤 작업을 의도하는지 나타내는 분류 결과다.
type Intent string

const (
	IntentHomeSearch      Intent = "home.search"
	IntentHomeAdd         Intent = "home.add"
	IntentHomeUpdate      Intent = "home.update"
	IntentHomeDelete      Intent = "home.delete"
	IntentKnowledgeUpdate Intent = "knowledge.update"
	IntentKnowledgeSearch Intent = "knowledge.search"
	IntentKnowledgeReview Intent = "knowledge.review"
	IntentSystemHelp      Intent = "system.help"
	IntentUnknown         Intent = "system.unknown"
)

// Namespace 는 intent 의 네임스페이스("home"/"knowledge"/"system")를 반환한다.
// 점이 없으면 전체 문자열을 그대로 반환한다.
func (i Intent) Namespace() string {
	s := string(i)
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// AllIntents 는 Phase 2 에서 분류 대상인 모든 유효 intent 목록을 반환한다.
// Gemini enum 제약과 응답 검증에서 공용으로 쓴다.
func AllIntents() []Intent {
	return []Intent{
		IntentHomeSearch,
		IntentHomeAdd,
		IntentHomeUpdate,
		IntentHomeDelete,
		IntentKnowledgeUpdate,
		IntentKnowledgeSearch,
		IntentKnowledgeReview,
		IntentSystemHelp,
		IntentUnknown,
	}
}

// Classifier 는 평문 텍스트를 Intent 로 분류하는 능력이다.
type Classifier interface {
	Classify(ctx context.Context, text string) (Intent, error)
}

// Worker 는 분류된 메시지를 처리해 Reply 를 생성하는 능력이다.
type Worker interface {
	Handle(ctx context.Context, intent Intent, in IncomingMessage) (Reply, error)
}

// MessageRouter 는 수신 메시지를 분류·디스패치해 Reply 를 만드는 능력이다.
type MessageRouter interface {
	Route(ctx context.Context, in IncomingMessage) (Reply, error)
}

// ChangeProposal 은 승인 대기 중인 변경안이다. 버튼 value 에 JSON 으로 인코딩된다.
type ChangeProposal struct {
	Action       string `json:"action"` // "add"
	ItemName     string `json:"item"`
	CategoryID   string `json:"category_id,omitempty"`   // resolve 된 Notion page ID
	CategoryName string `json:"category_name,omitempty"` // 표시용
	LocationID   string `json:"location_id"`             // resolve 된 Notion page ID
	LocationName string `json:"location_name"`           // 표시용
	Quantity     *int   `json:"quantity,omitempty"`
}

// ProposalApplier 는 승인된 변경안을 실제 시스템에 반영한다.
type ProposalApplier interface {
	Apply(ctx context.Context, p ChangeProposal) (Reply, error)
}
