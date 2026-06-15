package slack

import (
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	slackgo "github.com/slack-go/slack"
)

func TestBuildBlocks_textOnly(t *testing.T) {
	t.Parallel()
	blocks := buildBlocks(domain.Reply{Text: "안녕"})
	if len(blocks) != 1 {
		t.Fatalf("블록 수 = %d, want 1", len(blocks))
	}
	if _, ok := blocks[0].(*slackgo.SectionBlock); !ok {
		t.Fatalf("첫 블록이 section 이 아님: %T", blocks[0])
	}
}

func TestBuildBlocks_withButtons(t *testing.T) {
	t.Parallel()
	reply := domain.Reply{
		Text: "변경안",
		Buttons: []domain.Button{
			{Text: "승인", Action: "approve", Value: `{"item":"체온계"}`, Style: "primary"},
			{Text: "취소", Action: "cancel"},
		},
	}
	blocks := buildBlocks(reply)
	if len(blocks) != 2 {
		t.Fatalf("블록 수 = %d, want 2 (section+actions)", len(blocks))
	}
	action, ok := blocks[1].(*slackgo.ActionBlock)
	if !ok {
		t.Fatalf("둘째 블록이 action 이 아님: %T", blocks[1])
	}
	elems := action.Elements.ElementSet
	if len(elems) != 2 {
		t.Fatalf("버튼 수 = %d, want 2", len(elems))
	}
	btn0, ok := elems[0].(*slackgo.ButtonBlockElement)
	if !ok {
		t.Fatalf("첫 요소가 버튼이 아님: %T", elems[0])
	}
	if btn0.ActionID != "approve" || btn0.Value != `{"item":"체온계"}` {
		t.Fatalf("승인 버튼 ActionID=%q Value=%q", btn0.ActionID, btn0.Value)
	}
	if btn0.Style != slackgo.StylePrimary {
		t.Fatalf("승인 버튼 Style=%q", btn0.Style)
	}
}
