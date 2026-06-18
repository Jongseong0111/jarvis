// Package knowledge 는 ChatGPT 공유 대화를 추출·요약·저장한다.
package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"

// Conversation 은 공유 페이지에서 추출한 대화다(화자 구분 없는 거친 메시지 목록).
type Conversation struct {
	Title    string
	URL      string
	Messages []string
}

var (
	sharePat = regexp.MustCompile(`^https?://(chatgpt\.com|chat\.openai\.com)/share/`)
	titleRe  = regexp.MustCompile(`<title>([^<]*)</title>`)
	// 임베드 스트림에서 이스케이프 따옴표(\") 로 감싼 문자열. 내부에 따옴표/역슬래시 없는 구간만.
	// Go regexp 는 반복 횟수 상한이 1000 이므로 4000 대신 1000 사용.
	msgRe = regexp.MustCompile(`\\"([^"\\]{15,1000})\\"`)
)

// FetchConversation 은 공유 링크를 받아 대화를 추출한다(네트워크).
func FetchConversation(ctx context.Context, url string) (Conversation, error) {
	if !sharePat.MatchString(url) {
		return Conversation{}, fmt.Errorf("ChatGPT 공유 링크가 아니야: %s", url)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Conversation{}, err
	}
	req.Header.Set("User-Agent", browserUA)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Conversation{}, fmt.Errorf("공유 페이지 fetch 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Conversation{}, fmt.Errorf("공유 페이지 응답 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4MB
	if err != nil {
		return Conversation{}, err
	}
	return parseConversation(body, url)
}

// parseConversation 은 HTML 에서 제목과 대화 메시지를 거칠게 추출한다(순수 함수).
func parseConversation(html []byte, url string) (Conversation, error) {
	title := ""
	if m := titleRe.FindSubmatch(html); m != nil {
		title = strings.TrimSpace(string(m[1]))
		title = strings.TrimPrefix(title, "ChatGPT - ")
		title = strings.TrimSpace(strings.TrimPrefix(title, "ChatGPT"))
	}

	var msgs []string
	seen := map[string]bool{}
	total := 0
	for _, m := range msgRe.FindAllSubmatch(html, -1) {
		s := strings.TrimSpace(string(m[1]))
		if !looksLikeMessage(s) || seen[s] {
			continue
		}
		seen[s] = true
		msgs = append(msgs, s)
		total += len(s)
	}
	if len(msgs) == 0 || total < 100 {
		return Conversation{}, fmt.Errorf("대화를 추출하지 못했어")
	}
	return Conversation{Title: title, URL: url, Messages: msgs}, nil
}

// looksLikeMessage 는 추출 문자열이 (식별자/플래그가 아닌) 자연어 메시지인지 판별한다.
func looksLikeMessage(s string) bool {
	if len(s) < 15 || strings.HasPrefix(s, "http") {
		return false
	}
	if strings.ContainsRune(s, ' ') {
		return true
	}
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 { // 한글 음절
			return true
		}
	}
	return false
}
