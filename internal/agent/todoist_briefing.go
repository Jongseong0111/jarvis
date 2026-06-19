package agent

import (
	"context"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// formatBriefing 은 헤더 + 할일 목록을 텍스트로 만든다.
func formatBriefing(header string, tasks []todoist.Task) string {
	return header + "\n" + formatTaskLines(tasks)
}

// NewMorningBriefing 은 아침 브리핑 작업을 만든다(오늘+밀린, 빈 날도 안내 전송).
func NewMorningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		tasks, err := port.ListTasks(ctx, "today | overdue")
		if err != nil {
			log.FromContext(ctx).Error("아침 브리핑 조회 실패", "error", err)
			return
		}
		text := "☀️ 오늘 마감할 일이 없습니다. 좋은 하루 보내세요!"
		if len(tasks) > 0 {
			text = formatBriefing("☀️ *오늘 할 일과 밀린 일*", tasks)
		}
		sendText(ctx, sender, channel, text)
	}
}

// NewEveningBriefing 은 저녁 브리핑 작업을 만든다(오늘 미완료+내일, 빈 날 무음).
func NewEveningBriefing(port TodoistPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		tasks, err := port.ListTasks(ctx, "(today & !checked) | tomorrow")
		if err != nil {
			log.FromContext(ctx).Error("저녁 브리핑 조회 실패", "error", err)
			return
		}
		if len(tasks) == 0 {
			return // 무음
		}
		sendText(ctx, sender, channel, formatBriefing("🌙 *오늘 못 끝낸 일과 내일 할 일*", tasks))
	}
}

func sendText(ctx context.Context, sender domain.MessageSender, channel, text string) {
	if err := sender.Send(ctx, domain.Reply{ChannelID: channel, Text: text}); err != nil {
		log.FromContext(ctx).Error("브리핑 전송 실패", "error", err)
	}
}
