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
		text := "☀️ 할일 없어. 좋은 하루!"
		if len(tasks) > 0 {
			text = formatBriefing("☀️ 오늘 할일 + 밀린 거", tasks)
		}
		send(ctx, sender, channel, text)
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
		send(ctx, sender, channel, formatBriefing("🌙 오늘 미완료 / 내일 할일", tasks))
	}
}

func send(ctx context.Context, sender domain.MessageSender, channel, text string) {
	if err := sender.Send(ctx, domain.Reply{ChannelID: channel, Text: text}); err != nil {
		log.FromContext(ctx).Error("브리핑 전송 실패", "error", err)
	}
}
