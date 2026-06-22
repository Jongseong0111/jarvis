package gcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// testClient 는 httptest 서버에 붙은 Client 를 만든다. handler 가 응답을 결정한다.
func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	svc, err := calendar.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return newWithService(svc, "primary"), srv
}

func TestListEvents_ParsesTimedAndAllDay(t *testing.T) {
	t.Parallel()
	var gotPath, gotQuery string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(calendar.Events{Items: []*calendar.Event{
			{Id: "e1", Summary: "미팅", Location: "회의실",
				Start: &calendar.EventDateTime{DateTime: "2026-06-23T15:00:00+09:00"},
				End:   &calendar.EventDateTime{DateTime: "2026-06-23T16:00:00+09:00"}},
			{Id: "e2", Summary: "아기 검진",
				Start: &calendar.EventDateTime{Date: "2026-06-24"},
				End:   &calendar.EventDateTime{Date: "2026-06-25"}},
		}})
	})
	defer srv.Close()

	from := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	evs, err := c.ListEvents(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if !strings.Contains(gotPath, "/calendars/primary/events") {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "singleEvents=true") || !strings.Contains(gotQuery, "orderBy=startTime") {
		t.Fatalf("query = %q (singleEvents/orderBy 누락)", gotQuery)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}
	if evs[0].AllDay || evs[0].Summary != "미팅" || evs[0].Location != "회의실" {
		t.Fatalf("timed event 파싱 오류: %+v", evs[0])
	}
	if !evs[1].AllDay || evs[1].Summary != "아기 검진" {
		t.Fatalf("all-day event 파싱 오류: %+v", evs[1])
	}
}

func TestSearchEvents_SendsQuery(t *testing.T) {
	t.Parallel()
	var gotQuery string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(calendar.Events{Items: nil})
	})
	defer srv.Close()
	_, err := c.SearchEvents(context.Background(), "검진", time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SearchEvents: %v", err)
	}
	if !strings.Contains(gotQuery, "q=") || !strings.Contains(gotQuery, "%EA%B2%80%EC%A7%84") {
		t.Fatalf("query 에 q=검진 누락: %q", gotQuery)
	}
}

func TestAddEvent_Timed(t *testing.T) {
	t.Parallel()
	var body calendar.Event
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.Id = "new1"
		_ = json.NewEncoder(w).Encode(body)
	})
	defer srv.Close()
	start := time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC)
	out, err := c.AddEvent(context.Background(), Event{Summary: "회의", Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if out.ID != "new1" {
		t.Fatalf("반환 ID = %q", out.ID)
	}
	if body.Summary != "회의" || body.Start.DateTime == "" {
		t.Fatalf("요청 body 오류(타임드 DateTime 누락): %+v", body.Start)
	}
}

func TestAddEvent_AllDay(t *testing.T) {
	t.Parallel()
	var body calendar.Event
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(body)
	})
	defer srv.Close()
	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	_, err := c.AddEvent(context.Background(), Event{Summary: "검진", Start: day, End: day.AddDate(0, 0, 1), AllDay: true})
	if err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	if body.Start.Date != "2026-07-03" || body.Start.DateTime != "" {
		t.Fatalf("종일 이벤트는 Date 만 채워야 함: %+v", body.Start)
	}
}

func TestDeleteEvent(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath string
	c, srv := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()
	if err := c.DeleteEvent(context.Background(), "e9"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if gotMethod != http.MethodDelete || !strings.Contains(gotPath, "/events/e9") {
		t.Fatalf("delete 요청 오류: %s %s", gotMethod, gotPath)
	}
}
