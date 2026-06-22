package gcal

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

// OAuthConfig 는 캘린더용 OAuth2 설정을 만든다(cmd/calauth 와 공유).
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{calendar.CalendarEventsScope},
		RedirectURL:  redirectURL,
	}
}

// TokenSource 는 refresh token 으로 access token 을 자동 갱신하는 소스를 만든다.
func TokenSource(ctx context.Context, clientID, clientSecret, refreshToken string) oauth2.TokenSource {
	cfg := OAuthConfig(clientID, clientSecret, "")
	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
}
