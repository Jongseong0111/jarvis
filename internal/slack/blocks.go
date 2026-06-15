package slack

import (
	"github.com/Jongseong0111/jarvis/domain"
	slackgo "github.com/slack-go/slack"
)

// buildBlocks 는 Reply 를 Block Kit 블록으로 변환한다(텍스트 섹션 + 버튼 액션).
func buildBlocks(reply domain.Reply) []slackgo.Block {
	section := slackgo.NewSectionBlock(
		slackgo.NewTextBlockObject(slackgo.MarkdownType, reply.Text, false, false),
		nil, nil,
	)
	if len(reply.Buttons) == 0 {
		return []slackgo.Block{section}
	}

	elems := make([]slackgo.BlockElement, 0, len(reply.Buttons))
	for _, b := range reply.Buttons {
		// ActionID = 액션 종류("approve"/"cancel"), Value = 변경안 JSON
		btn := slackgo.NewButtonBlockElement(
			b.Action, b.Value,
			slackgo.NewTextBlockObject(slackgo.PlainTextType, b.Text, false, false),
		)
		if b.Style != "" {
			btn.Style = slackgo.Style(b.Style)
		}
		elems = append(elems, btn)
	}
	return []slackgo.Block{section, slackgo.NewActionBlock("", elems...)}
}
