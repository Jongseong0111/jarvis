package gcal

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const seoulTZName = "Asia/Seoul"

// Client 는 Google Calendar 호출을 감싼다.
type Client struct {
	svc        *calendar.Service
	calendarID string
}

// New 는 refresh token 기반 Client 를 만든다. calendarID 빈 값이면 "primary".
func New(ctx context.Context, clientID, clientSecret, refreshToken, calendarID string) (*Client, error) {
	svc, err := calendar.NewService(ctx, option.WithTokenSource(
		TokenSource(ctx, clientID, clientSecret, refreshToken)))
	if err != nil {
		return nil, fmt.Errorf("calendar 서비스 생성 실패: %w", err)
	}
	return newWithService(svc, calendarID), nil
}

func newWithService(svc *calendar.Service, calendarID string) *Client {
	if calendarID == "" {
		calendarID = "primary"
	}
	return &Client{svc: svc, calendarID: calendarID}
}

// ListEvents 는 [timeMin, timeMax) 의 일정을 시작시각 순으로 조회한다.
func (c *Client) ListEvents(ctx context.Context, timeMin, timeMax time.Time) ([]Event, error) {
	return c.list(ctx, "", timeMin, timeMax)
}

// SearchEvents 는 query 로 일정을 검색한다.
func (c *Client) SearchEvents(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error) {
	return c.list(ctx, query, timeMin, timeMax)
}

func (c *Client) list(ctx context.Context, query string, timeMin, timeMax time.Time) ([]Event, error) {
	call := c.svc.Events.List(c.calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime")
	if query != "" {
		call = call.Q(query)
	}
	res, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("일정 조회 실패: %w", err)
	}
	out := make([]Event, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, toEvent(it))
	}
	return out, nil
}

// AddEvent 는 일정을 등록하고 등록된 일정을 반환한다.
func (c *Client) AddEvent(ctx context.Context, ev Event) (Event, error) {
	created, err := c.svc.Events.Insert(c.calendarID, toCalendarEvent(ev)).Context(ctx).Do()
	if err != nil {
		return Event{}, fmt.Errorf("일정 등록 실패: %w", err)
	}
	return toEvent(created), nil
}

// DeleteEvent 는 일정을 삭제한다.
func (c *Client) DeleteEvent(ctx context.Context, id string) error {
	if err := c.svc.Events.Delete(c.calendarID, id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("일정 삭제 실패: %w", err)
	}
	return nil
}

// toEvent 는 SDK 이벤트를 도메인 Event 로 변환한다(타임드/종일 구분).
func toEvent(e *calendar.Event) Event {
	ev := Event{ID: e.Id, Summary: e.Summary, Location: e.Location}
	if e.Start != nil {
		if e.Start.Date != "" { // 종일
			ev.AllDay = true
			ev.Start, _ = time.Parse("2006-01-02", e.Start.Date)
			if e.End != nil && e.End.Date != "" {
				ev.End, _ = time.Parse("2006-01-02", e.End.Date)
			}
		} else {
			ev.Start, _ = time.Parse(time.RFC3339, e.Start.DateTime)
			if e.End != nil && e.End.DateTime != "" {
				ev.End, _ = time.Parse(time.RFC3339, e.End.DateTime)
			}
		}
	}
	return ev
}

// toCalendarEvent 는 도메인 Event 를 SDK 등록용으로 변환한다.
func toCalendarEvent(ev Event) *calendar.Event {
	out := &calendar.Event{Summary: ev.Summary, Location: ev.Location}
	if ev.AllDay {
		out.Start = &calendar.EventDateTime{Date: ev.Start.Format("2006-01-02")}
		out.End = &calendar.EventDateTime{Date: ev.End.Format("2006-01-02")}
	} else {
		out.Start = &calendar.EventDateTime{DateTime: ev.Start.Format(time.RFC3339), TimeZone: seoulTZName}
		out.End = &calendar.EventDateTime{DateTime: ev.End.Format(time.RFC3339), TimeZone: seoulTZName}
	}
	return out
}
