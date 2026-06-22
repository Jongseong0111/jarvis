package devdigest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleRSS = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item><title>Go 1.25 출시</title><link>https://go.dev/blog/go1.25</link><description>새 기능 요약</description></item>
  <item><title>Rust 2025 에디션</title><link>https://blog.rust-lang.org/2025</link><description>Rust 업데이트</description></item>
</channel></rss>`

// sampleAtom 은 GeekNews 와 같은 Atom 포맷이다(링크는 href 속성, 본문은 content HTML).
const sampleAtom = `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns='http://www.w3.org/2005/Atom'>
<title>GeekNews</title>
<entry>
  <title><![CDATA[ponytail - 게으른 시니어처럼]]></title>
  <link rel='alternate' type='text/html' href='https://news.hada.io/topic?id=30701' />
  <id>https://news.hada.io/topic?id=30701</id>
  <content type='html'><![CDATA[<blockquote><p>최고의 코드는 <strong>작성하지 않은</strong> 코드</p></blockquote>]]></content>
</entry>
</feed>`

func newTestFetcher(rssURLs []string, hnTopURL, hnItemBase string) *MultiFetcher {
	return &MultiFetcher{
		httpClient:    &http.Client{},
		rssURLs:       rssURLs,
		hnTopURL:      hnTopURL,
		hnItemBaseURL: hnItemBase,
	}
}

func TestFetcher_RSS(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, sampleRSS)
	}))
	defer srv.Close()

	f := newTestFetcher([]string{srv.URL}, "invalid://hn-top", "invalid://hn-item/%d")
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("기대 2건, got %d: %+v", len(items), items)
	}
	if items[0].Title != "Go 1.25 출시" || items[0].URL != "https://go.dev/blog/go1.25" {
		t.Fatalf("첫 아이템 불일치: %+v", items[0])
	}
	// rssURLs[0] 은 GeekNews 로 라벨링되어야 한다.
	if items[0].Source != "GeekNews" {
		t.Fatalf("Source=GeekNews 기대: %+v", items[0])
	}
}

func TestFetcher_Atom(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sampleAtom)
	}))
	defer srv.Close()

	f := newTestFetcher([]string{srv.URL}, "invalid://hn-top", "invalid://hn-item/%d")
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Atom 1건 기대, got %d: %+v", len(items), items)
	}
	it := items[0]
	if it.Title != "ponytail - 게으른 시니어처럼" {
		t.Fatalf("제목 불일치: %q", it.Title)
	}
	// 링크는 href 속성에서 가져와야 한다.
	if it.URL != "https://news.hada.io/topic?id=30701" {
		t.Fatalf("URL 불일치: %q", it.URL)
	}
	// 본문 HTML 태그는 제거되어야 한다.
	if strings.Contains(it.Desc, "<") || !strings.Contains(it.Desc, "작성하지 않은") {
		t.Fatalf("HTML 정리 실패: %q", it.Desc)
	}
	if it.Source != "GeekNews" {
		t.Fatalf("Source=GeekNews 기대: %+v", it)
	}
}

func TestCleanDesc(t *testing.T) {
	t.Parallel()
	got := cleanDesc("<p>안녕 &amp; <strong>반가워</strong></p>")
	if got != "안녕 & 반가워" {
		t.Fatalf("cleanDesc=%q", got)
	}
}

func TestFetcher_HN(t *testing.T) {
	t.Parallel()

	itemSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1,"title":"HN 기사","url":"https://example.com","type":"story","score":100}`)
	}))
	defer itemSrv.Close()

	topSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[1]`)
	}))
	defer topSrv.Close()

	f := newTestFetcher(nil, topSrv.URL, itemSrv.URL+"/%d")
	items, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Title == "HN 기사" {
			found = true
			if it.Source != "HN" {
				t.Fatalf("Source=HN 기대: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("HN 아이템 없음: %+v", items)
	}
}

func TestFetcher_AllSourcesFail(t *testing.T) {
	t.Parallel()
	f := newTestFetcher([]string{"http://127.0.0.1:1"}, "http://127.0.0.1:1", "http://127.0.0.1:1/%d")
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Fatal("모든 소스 실패 시 error 기대")
	}
}
