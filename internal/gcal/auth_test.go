package gcal

import (
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestOAuthConfig(t *testing.T) {
	t.Parallel()
	cfg := OAuthConfig("cid", "secret", "http://localhost:8910")
	if cfg.ClientID != "cid" || cfg.ClientSecret != "secret" {
		t.Fatalf("client id/secret 미설정: %+v", cfg)
	}
	if cfg.RedirectURL != "http://localhost:8910" {
		t.Fatalf("redirect url = %q", cfg.RedirectURL)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != calendar.CalendarEventsScope {
		t.Fatalf("scope = %v, want [%s]", cfg.Scopes, calendar.CalendarEventsScope)
	}
	if !strings.Contains(cfg.Endpoint.AuthURL, "google") {
		t.Fatalf("endpoint AuthURL = %q (google 아님)", cfg.Endpoint.AuthURL)
	}
}
