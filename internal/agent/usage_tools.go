package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/internal/usage"
)

// UsageReader 는 비용 집계를 읽는 능력이다(usage.Recorder 가 구현).
type UsageReader interface {
	Query(from, to time.Time) (usage.Summary, error)
}

// UsageTools 는 비용 조회 도구(list_usage)를 만든다.
func UsageTools(r UsageReader) []Tool {
	return []Tool{{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_usage",
			Description: "기간별 LLM 사용 비용을 조회한다(오늘/이번주/이번달). 사용자가 비용·요금을 물으면 사용.",
			Parameters: objSchema(map[string]*genai.Schema{
				"period": strSchema("기간: today(기본), week, month 중 하나"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			period := strArg(args, "period")
			if period == "" {
				period = "today"
			}
			// 기록은 Asia/Seoul(+09:00)로 남으므로 기간 경계도 서울 기준으로 잡는다(호스트 TZ 무관).
			from, to := usage.RangeForPeriod(time.Now().In(seoulLoc()), period)
			s, err := r.Query(from, to)
			if err != nil {
				return "", fmt.Errorf("비용 조회 실패: %w", err)
			}
			return formatUsage(period, s), nil
		},
	}}
}

func periodLabel(period string) string {
	switch period {
	case "week":
		return "이번주"
	case "month":
		return "이번달"
	default:
		return "오늘"
	}
}

// formatUsage 는 집계를 존댓말+이모지+• 불릿으로 포맷한다.
func formatUsage(period string, s usage.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💰 %s LLM 비용: $%.4f (%d회 호출)\n", periodLabel(period), s.TotalCost, s.TotalCalls)
	if s.TotalCalls == 0 {
		return b.String() + "\n아직 기록된 호출이 없습니다."
	}
	b.WriteString("\n*모델별*\n")
	for _, m := range s.ByModel {
		fmt.Fprintf(&b, "• %s: $%.4f (%d회)\n", m.Key, m.CostUSD, m.Calls)
	}
	b.WriteString("\n*기능별*\n")
	for _, f := range s.ByFeature {
		fmt.Fprintf(&b, "• %s: $%.4f (%d회)\n", f.Key, f.CostUSD, f.Calls)
	}
	return strings.TrimRight(b.String(), "\n")
}
