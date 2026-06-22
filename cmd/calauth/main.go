// Command calauth 는 Google Calendar refresh token 을 1회 발급한다.
// 사용법: config/.env 에 GOOGLE_OAUTH_CLIENT_ID/SECRET 기입 후 `go run ./cmd/calauth`.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"

	"github.com/Jongseong0111/jarvis/internal/gcal"
)

const redirectURL = "http://localhost:8910"

func main() {
	_ = godotenv.Load("config/.env")
	clientID := os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	secret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	if clientID == "" || secret == "" {
		log.Fatal("GOOGLE_OAUTH_CLIENT_ID / GOOGLE_OAUTH_CLIENT_SECRET 를 config/.env 에 먼저 넣어주세요.")
	}

	cfg := gcal.OAuthConfig(clientID, secret, redirectURL)
	state, err := randomState()
	if err != nil {
		log.Fatalf("state 생성 실패: %v", err)
	}

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8910"}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state 불일치", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		fmt.Fprintln(w, "인증 완료. 터미널로 돌아가세요.")
		codeCh <- code
	})
	go func() { _ = srv.ListenAndServe() }()

	// offline+consent 옵션으로 refresh token 을 반드시 발급받는다.
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("아래 URL 을 브라우저에서 열어 로그인/동의하세요:")
	fmt.Println(authURL)

	code := <-codeCh
	_ = srv.Shutdown(context.Background())

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("토큰 교환 실패: %v", err)
	}
	if tok.RefreshToken == "" {
		log.Fatal("refresh token 이 비었습니다. 동의 화면에서 offline access 를 허용했는지 확인하세요.")
	}
	fmt.Println("\n✅ 아래를 config/.env 에 추가하세요:")
	fmt.Printf("GOOGLE_CALENDAR_REFRESH_TOKEN=%s\n", tok.RefreshToken)
}

// randomState 는 CSRF 방지용 무작위 state 를 만든다.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
