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
		// localhost 1회용 도구라 CSRF state 검증은 생략한다(예전 탭/캐시 콜백으로 인한 마찰 방지).
		// code 가 있는 콜백만 받아 처리하고, 거부/빈 콜백은 무시하고 계속 대기한다.
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "code 없음(거부됐거나 잘못된 콜백). 진짜 동의 콜백을 기다립니다.", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "인증 완료. 터미널로 돌아가세요.")
		codeCh <- code
	})
	go func() { _ = srv.ListenAndServe() }()

	// offline+consent 옵션으로 refresh token 을 반드시 발급받는다.
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("아래 URL 을 브라우저에서 열어 로그인/동의하세요:")
	fmt.Println(authURL)

	code := <-codeCh
	if code == "" {
		_ = srv.Shutdown(context.Background())
		log.Fatal("인증이 취소되었거나 실패했습니다. 다시 시도해주세요.")
	}
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
