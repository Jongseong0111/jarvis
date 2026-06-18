package knowledge

import (
	"os"
	"strings"
	"testing"
)

func TestParseConversation_extractsTitleAndMessages(t *testing.T) {
	t.Parallel()
	html, err := os.ReadFile("testdata/share-sample.html")
	if err != nil {
		t.Fatalf("픽스처 읽기: %v", err)
	}
	conv, err := parseConversation(html, "https://chatgpt.com/share/abc")
	if err != nil {
		t.Fatalf("parseConversation: %v", err)
	}
	if conv.Title != "고랭 장점 설명" {
		t.Fatalf("제목 = %q, want 고랭 장점 설명", conv.Title)
	}
	joined := strings.Join(conv.Messages, "\n")
	if !strings.Contains(joined, "고루틴") || !strings.Contains(joined, "워커 풀") {
		t.Fatalf("핵심 메시지 누락: %q", joined)
	}
	for _, m := range conv.Messages {
		if strings.Contains(m, "experiment_enabled") || strings.Contains(m, "stream_impl") {
			t.Fatalf("식별자 잡음이 메시지로 추출됨: %q", m)
		}
	}
}

func TestParseConversation_emptyErrors(t *testing.T) {
	t.Parallel()
	if _, err := parseConversation([]byte("<title>빈 페이지</title><body>no data</body>"), "u"); err == nil {
		t.Fatal("대화 없는 페이지는 에러여야 함")
	}
}

func TestLooksLikeMessage(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"고루틴은 경량 동시성 단위야. 수를 제한해.":                        true,  // 한글 + 길이
		"worker pool limits concurrent goroutines safely": true,  // 영문 문장(공백)
		"update_custom_instructions_beacon_enabled":       false, // 식별자(공백·한글 없음)
		"short": false, // 너무 짧음
		"https://example.com/some/long/path/here": false, // URL
	}
	for in, want := range cases {
		if got := looksLikeMessage(in); got != want {
			t.Errorf("looksLikeMessage(%q) = %v, want %v", in, got, want)
		}
	}
}
