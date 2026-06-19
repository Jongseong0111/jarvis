package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/devdigest"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// NewDevDigestBriefing 은 개발 다이제스트 브리핑 스케줄러 잡을 만든다.
// fetch 실패 시 빈 목록으로 generate 를 시도한다. generate 실패 시 무음.
func NewDevDigestBriefing(fetcher devdigest.Fetcher, generator devdigest.Generator, sender domain.MessageSender, channel string) func(ctx context.Context) {
	return func(ctx context.Context) {
		items, err := fetcher.Fetch(ctx)
		if err != nil {
			log.FromContext(ctx).Error("뉴스 fetch 실패", "error", err)
			// 빈 items 로 공부주제만이라도 생성 시도
		}

		result, err := generator.Generate(ctx, items)
		if err != nil {
			log.FromContext(ctx).Error("다이제스트 생성 실패", "error", err)
			return
		}

		sendText(ctx, sender, channel, formatDigest(result))
	}
}

func formatDigest(r devdigest.DigestResult) string {
	var sb strings.Builder

	sb.WriteString("📰 *오늘의 개발 소식*\n")
	for _, n := range r.News {
		sb.WriteString(fmt.Sprintf("• <%s|%s> — %s\n", n.URL, n.Title, n.Summary))
	}

	sb.WriteString(fmt.Sprintf("\n📚 *오늘의 공부 주제*  _(도메인: %s)_\n", r.Domain))
	for _, topic := range r.Topics {
		sb.WriteString("• " + topic + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
