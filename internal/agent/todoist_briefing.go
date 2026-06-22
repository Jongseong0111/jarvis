package agent

import (
	"context"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/todoist"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// formatBriefing 은 헤더 + 할일 목록을 텍스트로 만든다.
func formatBriefing(header string, tasks []todoist.Task) string {
	return header + "\n" + formatTaskLines(tasks)
}

// NewMorningBriefing 은 아침 브리핑 작업을 만든다(오늘 일정 + 오늘·밀린 할일).
// cal 이 nil 이면 할일만(기존 동작).
func NewMorningBriefing(port TodoistPort, cal CalendarPort, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		var sections []string
		if evLines, ok := todayEventLines(ctx, cal); ok {
			sections = append(sections, "📅 *오늘 일정*\n"+evLines)
		}
		tasks, err := port.ListTasks(ctx, "today | overdue")
		if err != nil {
			log.FromContext(ctx).Error("아침 브리핑 조회 실패", "error", err)
			if len(sections) == 0 {
				return // 할일 조회 실패 + 일정 없음 → 무음(원본 동작 보존)
			}
		} else if len(tasks) > 0 {
			sections = append(sections, "☀️ *오늘 할 일과 밀린 일*\n"+formatTaskLines(tasks))
		}
		if len(sections) == 0 {
			sendText(ctx, sender, channel, "☀️ 오늘 마감할 일이 없습니다. 좋은 하루 보내세요!")
			return
		}
		sendText(ctx, sender, channel, strings.Join(sections, "\n\n"))
	}
}

// todayEventLines 는 오늘 일정을 • 줄로 만든다. cal nil/오류면 ("", false)(best-effort).
func todayEventLines(ctx context.Context, cal CalendarPort) (string, bool) {
	if cal == nil {
		return "", false
	}
	mn, mx := eventRange(calNow(), "today")
	evs, err := cal.ListEvents(ctx, mn, mx)
	if err != nil {
		log.FromContext(ctx).Error("브리핑 일정 조회 실패", "error", err)
		return "", false
	}
	if len(evs) == 0 {
		return "", false
	}
	return formatEvents(evs), true
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
