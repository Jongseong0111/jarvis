package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/gcal"
)

// calNow 는 캘린더 도구가 쓰는 현재 시각이다(테스트에서 교체).
var calNow = time.Now

// seoulLoc 는 Asia/Seoul 위치를 반환한다(tzdata 없으면 +09:00 고정).
func seoulLoc() *time.Location {
	if loc, err := time.LoadLocation("Asia/Seoul"); err == nil {
		return loc
	}
	return time.FixedZone("Asia/Seoul", 9*60*60)
}

// CalendarPort 는 캘린더 조작 능력이다(gcal.Client 가 구현).
type CalendarPort interface {
	ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]gcal.Event, error)
	SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]gcal.Event, error)
	AddEvent(ctx context.Context, ev gcal.Event) (gcal.Event, error)
	DeleteEvent(ctx context.Context, id string) error
}

type calendarTools struct{ port CalendarPort }

// CalendarTools 는 일정 도구 목록을 만든다.
// list/add/search 는 즉시 실행(Run), delete 만 변경안(Propose).
func CalendarTools(port CalendarPort) []Tool {
	t := calendarTools{port: port}
	return []Tool{t.listEvents(), t.addEvent(), t.searchEvents(), t.deleteEvent()}
}

func (t calendarTools) listEvents() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_events",
			Description: "캘린더 일정을 조회한다. period: today, tomorrow, week(기본), month.",
			Parameters: objSchema(map[string]*genai.Schema{
				"period": strSchema("today/tomorrow/week/month 중 하나(기본 week)"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			period := strings.TrimSpace(strArg(args, "period"))
			if period == "" {
				period = "week"
			}
			mn, mx := eventRange(calNow(), period)
			evs, err := t.port.ListEvents(ctx, mn, mx)
			if err != nil {
				return "", err
			}
			if len(evs) == 0 {
				return "📅 해당 기간에 일정이 없습니다.", nil
			}
			return "📅 *일정*\n" + formatEvents(evs), nil
		},
	}
}

func (t calendarTools) addEvent() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "add_event",
			Description: "캘린더에 일정을 추가한다. start 는 RFC3339(예: 2026-06-29T15:00:00+09:00) 또는 종일이면 YYYY-MM-DD. end 는 선택.",
			Parameters: objSchema(map[string]*genai.Schema{
				"summary":  strSchema("일정 제목"),
				"start":    strSchema("시작. RFC3339 또는 YYYY-MM-DD(종일)"),
				"end":      strSchema("종료(선택). 형식은 start 와 동일"),
				"location": strSchema("장소(선택)"),
			}, "summary", "start"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			summary := strings.TrimSpace(strArg(args, "summary"))
			if summary == "" {
				return "", fmt.Errorf("일정 제목을 알려주세요.")
			}
			start, end, allDay, err := parseEventTimes(strArg(args, "start"), strArg(args, "end"))
			if err != nil {
				return "", err
			}
			ev, err := t.port.AddEvent(ctx, gcal.Event{
				Summary: summary, Start: start, End: end, AllDay: allDay,
				Location: strings.TrimSpace(strArg(args, "location")),
			})
			if err != nil {
				return "", err
			}
			return "✅ 일정을 추가했습니다.\n" + formatEventLine(ev), nil
		},
	}
}

func (t calendarTools) searchEvents() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "search_events",
			Description: "키워드로 일정을 검색한다(과거~미래). 예: '아기 검진 언제였지?'",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("검색어"),
			}, "query"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			q := strings.TrimSpace(strArg(args, "query"))
			if q == "" {
				return "", fmt.Errorf("검색어를 알려주세요.")
			}
			now := calNow()
			evs, err := t.port.SearchEvents(ctx, q, now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0))
			if err != nil {
				return "", err
			}
			if len(evs) == 0 {
				return fmt.Sprintf("🔍 '%s' 일정을 찾지 못했습니다.", q), nil
			}
			return "🔍 *검색 결과*\n" + formatEvents(evs), nil
		},
	}
}

func (t calendarTools) deleteEvent() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_event",
			Description: "일정을 삭제한다. query 로 대상을 찾아 승인 버튼을 거친다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"query": strSchema("삭제할 일정 키워드(부분일치)"),
			}, "query"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			ev, err := t.resolveEvent(ctx, strArg(args, "query"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			return domain.ChangeProposal{
				Op:      "delete_event",
				Summary: "🗑️ 다음 일정을 삭제할까요?\n" + formatEventLine(ev),
				Fields:  map[string]string{"event_id": ev.ID, "summary": ev.Summary},
			}, nil
		},
	}
}

// resolveEvent 는 query 로 일정 1개를 찾는다(최근 30일~+1년). 0개/다수면 에러(되묻기).
func (t calendarTools) resolveEvent(ctx context.Context, query string) (gcal.Event, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return gcal.Event{}, fmt.Errorf("어떤 일정인지 알려주세요.")
	}
	now := calNow()
	evs, err := t.port.SearchEvents(ctx, query, now.AddDate(0, 0, -30), now.AddDate(1, 0, 0))
	if err != nil {
		return gcal.Event{}, err
	}
	switch len(evs) {
	case 0:
		return gcal.Event{}, fmt.Errorf("'%s'에 해당하는 일정을 찾지 못했습니다.", query)
	case 1:
		return evs[0], nil
	default:
		var names []string
		for _, e := range evs {
			names = append(names, e.Summary)
		}
		return gcal.Event{}, fmt.Errorf("'%s'에 해당하는 일정이 여러 개예요: %s. 더 정확히 알려주세요.", query, strings.Join(names, ", "))
	}
}

// eventRange 는 period 에 대한 [timeMin, timeMax) 를 now 기준으로 만든다.
// 날짜 경계는 호스트 TZ 에 무관하게 항상 Asia/Seoul 자정 기준이다.
func eventRange(now time.Time, period string) (timeMin, timeMax time.Time) {
	now = now.In(seoulLoc())
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, seoulLoc())
	switch period {
	case "today":
		return start, start.AddDate(0, 0, 1)
	case "tomorrow":
		return start.AddDate(0, 0, 1), start.AddDate(0, 0, 2)
	case "month":
		return start, start.AddDate(0, 0, 30)
	default: // week
		return start, start.AddDate(0, 0, 7)
	}
}

// parseEventTimes 는 start/end 문자열을 파싱한다. YYYY-MM-DD 면 종일, RFC3339 면 타임드.
// end 가 비면 타임드는 +1h, 종일은 +1일.
func parseEventTimes(startStr, endStr string) (start, end time.Time, allDay bool, err error) {
	startStr = strings.TrimSpace(startStr)
	endStr = strings.TrimSpace(endStr)
	if startStr == "" {
		return start, end, false, fmt.Errorf("시작 시각을 알려주세요.")
	}
	if len(startStr) == 10 { // YYYY-MM-DD
		allDay = true
		start, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			return start, end, false, fmt.Errorf("날짜 형식 오류(YYYY-MM-DD): %q", startStr)
		}
		if endStr != "" {
			end, err = time.Parse("2006-01-02", endStr)
			if err != nil {
				return start, end, false, fmt.Errorf("종료 날짜 형식 오류: %q", endStr)
			}
		} else {
			end = start.AddDate(0, 0, 1)
		}
		return start, end, true, nil
	}
	start, err = time.Parse(time.RFC3339, startStr)
	if err != nil {
		return start, end, false, fmt.Errorf("시작 시각 형식 오류(RFC3339): %q", startStr)
	}
	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return start, end, false, fmt.Errorf("종료 시각 형식 오류: %q", endStr)
		}
	} else {
		end = start.Add(time.Hour)
	}
	return start, end, false, nil
}

// formatEvents 는 일정 목록을 • 불릿으로 만든다.
func formatEvents(evs []gcal.Event) string {
	var lines []string
	for _, e := range evs {
		lines = append(lines, formatEventLine(e))
	}
	return strings.Join(lines, "\n")
}

// formatEventLine 은 일정 1건을 한 줄로 만든다(Asia/Seoul 표기).
// tzdata 없는 환경에서도 seoulLoc() 이 +09:00 을 보장한다(UTC 폴백 금지 — agent.go 컨벤션).
func formatEventLine(e gcal.Event) string {
	s := e.Start.In(seoulLoc())
	when := s.Format("1월 2일 (Mon) 15:04")
	if e.AllDay {
		when = s.Format("1월 2일") + " (종일)"
	}
	line := "• " + when + " — " + e.Summary
	if e.Location != "" {
		line += " @" + e.Location
	}
	return line
}
