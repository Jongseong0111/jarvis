package domain

import (
	"context"
	"encoding/json"
)

// MessageRouter 는 수신 메시지를 받아 Reply 를 만드는 능력이다(에이전트가 구현).
type MessageRouter interface {
	Route(ctx context.Context, in IncomingMessage) (Reply, error)
}

// ChangeProposal 은 승인 대기 중인 변경안이다. 버튼 value 에 JSON 으로 인코딩된다.
// Op 로 동작을 구분하고, Fields(단건) 또는 Items(일괄)에 resolved 값을 담는다.
type ChangeProposal struct {
	Op      string              `json:"op"`               // "add_item"|"add_items"|"update_item"|"delete_item"|"add_location"|"delete_location"
	Summary string              `json:"summary"`          // 버튼 메시지 본문(사람용 요약)
	Fields  map[string]string   `json:"fields,omitempty"` // 단건 op 의 resolved 값
	Items   []map[string]string `json:"items,omitempty"`  // 일괄 op(add_items)의 항목들
}

// ProposalApplier 는 승인된 변경안을 실제 시스템에 반영한다.
type ProposalApplier interface {
	Apply(ctx context.Context, p ChangeProposal) (Reply, error)
}

// Encode 는 변경안을 버튼 value 용 JSON 문자열로 직렬화한다.
func (p ChangeProposal) Encode() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// DecodeProposal 은 버튼 value 문자열을 변경안으로 역직렬화한다.
func DecodeProposal(s string) (ChangeProposal, error) {
	var p ChangeProposal
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return ChangeProposal{}, err
	}
	return p, nil
}
