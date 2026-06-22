// Package devdigest 는 개발자 아침 다이제스트(뉴스+공부주제)를 생성한다.
package devdigest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultGeekNewsRSS = "https://news.hada.io/rss/news"
	defaultHNTopURL    = "https://hacker-news.firebaseio.com/v0/topstories.json"
	defaultHNItemBase  = "https://hacker-news.firebaseio.com/v0/item/%d.json"
	maxHNFetch         = 30
	maxRSSItemsPerFeed = 30
	maxDescRunes       = 200 // 후보 설명 최대 길이(HTML 본문 절단)
)

// NewsItem 은 뉴스 피드에서 가져온 기사 하나다.
type NewsItem struct {
	Title  string
	URL    string
	Desc   string
	Source string // 출처 라벨(예: "GeekNews", "HN", "RSS")
}

// Fetcher 는 뉴스 아이템을 가져오는 인터페이스다.
type Fetcher interface {
	Fetch(ctx context.Context) ([]NewsItem, error)
}

// MultiFetcher 는 RSS 피드들 + HN API 에서 뉴스를 병렬로 가져온다.
type MultiFetcher struct {
	httpClient    *http.Client
	rssURLs       []string
	hnTopURL      string
	hnItemBaseURL string
}

// NewFetcher 는 GeekNews RSS + HN + 추가 RSS URL 로 MultiFetcher 를 생성한다.
func NewFetcher(extraURLs []string) *MultiFetcher {
	rssURLs := append([]string{defaultGeekNewsRSS}, extraURLs...)
	return &MultiFetcher{
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		rssURLs:       rssURLs,
		hnTopURL:      defaultHNTopURL,
		hnItemBaseURL: defaultHNItemBase,
	}
}

// Fetch 는 등록된 모든 소스에서 뉴스를 병렬로 가져온다.
// 개별 소스 실패는 건너뛴다. 모든 소스가 실패하면 error 를 반환한다.
func (f *MultiFetcher) Fetch(ctx context.Context) ([]NewsItem, error) {
	var (
		mu    sync.Mutex
		items []NewsItem
		errs  []error
		wg    sync.WaitGroup
	)

	collect := func(got []NewsItem, err error, label string) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
			return
		}
		items = append(items, got...)
	}

	// rssURLs[0] 은 GeekNews(NewFetcher 가 맨 앞에 둠), 나머지는 일반 RSS 로 라벨링한다.
	for i, u := range f.rssURLs {
		source := "RSS"
		if i == 0 {
			source = "GeekNews"
		}
		wg.Add(1)
		go func(url, src string) {
			defer wg.Done()
			got, err := f.fetchRSS(ctx, url, src)
			collect(got, err, url)
		}(u, source)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err := f.fetchHN(ctx)
		collect(got, err, "hn")
	}()

	wg.Wait()

	if len(items) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("모든 뉴스 소스 fetch 실패: %v", errs)
	}
	return items, nil
}

// feedXML 은 RSS 2.0(channel>item)과 Atom(feed>entry) 양쪽을 함께 파싱한다.
// GeekNews 는 Atom, 일반 RSS 는 channel>item 을 채운다.
type feedXML struct {
	RSSItems []struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
	} `xml:"channel>item"`
	AtomEntries []struct {
		Title string `xml:"title"`
		Links []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
			Type string `xml:"type,attr"`
		} `xml:"link"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
	} `xml:"entry"`
}

func (f *MultiFetcher) fetchRSS(ctx context.Context, url, source string) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed feedXML
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("피드 파싱 실패: %w", err)
	}

	var out []NewsItem
	// RSS 2.0
	for _, it := range feed.RSSItems {
		if len(out) >= maxRSSItemsPerFeed {
			break
		}
		out = append(out, NewsItem{Title: it.Title, URL: it.Link, Desc: cleanDesc(it.Description), Source: source})
	}
	// Atom (GeekNews 등)
	for _, e := range feed.AtomEntries {
		if len(out) >= maxRSSItemsPerFeed {
			break
		}
		desc := e.Summary
		if strings.TrimSpace(desc) == "" {
			desc = e.Content
		}
		out = append(out, NewsItem{Title: strings.TrimSpace(e.Title), URL: atomLink(e.Links), Desc: cleanDesc(desc), Source: source})
	}
	return out, nil
}

// atomLink 은 Atom entry 의 링크들 중 본문(text/html, rel=alternate) 링크를 고른다.
func atomLink(links []struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Type == "text/html" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// cleanDesc 는 HTML 태그를 제거하고 공백을 정리한 뒤 maxDescRunes 로 절단한다.
func cleanDesc(s string) string {
	s = htmlTagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxDescRunes {
		return string(r[:maxDescRunes]) + "…"
	}
	return s
}

func (f *MultiFetcher) fetchHN(ctx context.Context) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.hnTopURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, fmt.Errorf("HN top stories 파싱 실패: %w", err)
	}
	if len(ids) > maxHNFetch {
		ids = ids[:maxHNFetch]
	}

	var (
		mu    sync.Mutex
		items []NewsItem
		wg    sync.WaitGroup
	)
	for _, id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			it, err := f.fetchHNItem(ctx, id)
			if err != nil || it == nil {
				return
			}
			mu.Lock()
			items = append(items, *it)
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return items, nil
}

type hnItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

func (f *MultiFetcher) fetchHNItem(ctx context.Context, id int) (*NewsItem, error) {
	url := fmt.Sprintf(f.hnItemBaseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var it hnItem
	if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
		return nil, err
	}
	if it.Type != "story" || it.URL == "" {
		return nil, nil // 링크 없는 Ask HN 등 스킵
	}
	return &NewsItem{Title: it.Title, URL: it.URL, Source: "HN"}, nil
}
