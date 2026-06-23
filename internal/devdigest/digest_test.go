package devdigest

import (
	"strings"
	"testing"
)

func TestParseResponse_valid(t *testing.T) {
	t.Parallel()
	raw := `{"news":[{"title":"Go 1.25","url":"https://go.dev","summary":"새 기능 출시"}],"domain":"언어","topics":["언어 → Go → goroutine 스케줄러"]}`
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "언어" {
		t.Fatalf("domain=%q", result.Domain)
	}
	if len(result.News) != 1 || result.News[0].URL != "https://go.dev" {
		t.Fatalf("news=%+v", result.News)
	}
	if len(result.Topics) != 1 || !strings.Contains(result.Topics[0], "goroutine") {
		t.Fatalf("topics=%+v", result.Topics)
	}
}

func TestParseResponse_markdownWrapped(t *testing.T) {
	t.Parallel()
	raw := "```json\n{\"news\":[],\"domain\":\"AI\",\"topics\":[\"AI → LLM → Transformer 구조\"]}\n```"
	result, err := parseResponse(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if result.Domain != "AI" {
		t.Fatalf("domain=%q", result.Domain)
	}
}

func TestParseResponse_invalid(t *testing.T) {
	t.Parallel()
	_, err := parseResponse("not json")
	if err == nil {
		t.Fatal("잘못된 JSON 에 error 기대")
	}
}

func TestBuildPrompt_geekNewsInPoolWithInstruction(t *testing.T) {
	t.Parallel()
	items := []NewsItem{
		{Title: "긱뉴스글", URL: "https://hada.io/1", Desc: "긱설명", Source: "GeekNews"},
		{Title: "HN글", URL: "https://hn.com/2", Desc: "HN설명", Source: "HN"},
	}
	p := buildPrompt(items, true, "언어")
	// GeekNews와 HN 모두 후보 풀에 라벨과 함께 남아야 한다.
	if !strings.Contains(p, "[GeekNews]") || !strings.Contains(p, "긱뉴스글") {
		t.Fatalf("GeekNews가 후보에 없음: %q", p)
	}
	if !strings.Contains(p, "[HN]") || !strings.Contains(p, "HN글") {
		t.Fatalf("HN이 후보에 없음: %q", p)
	}
	// GeekNews 1~2개 + 다른 출처 혼합 지시가 있어야 한다.
	if !strings.Contains(p, "1~2개") || !strings.Contains(p, "치우치지 마라") {
		t.Fatalf("GeekNews 혼합 지시 없음: %q", p)
	}
}

func TestBuildPrompt_noGeekNewsNoInstruction(t *testing.T) {
	t.Parallel()
	items := []NewsItem{{Title: "HN글", URL: "https://hn.com/2", Source: "HN"}}
	p := buildPrompt(items, false, "언어")
	if strings.Contains(p, "반드시 포함") {
		t.Fatalf("GeekNews 없는데 포함 지시가 생김: %q", p)
	}
	if !strings.Contains(p, "HN글") {
		t.Fatalf("HN 후보 없음: %q", p)
	}
}

func TestGeekNewsItems(t *testing.T) {
	t.Parallel()
	items := []NewsItem{
		{Title: "G1", URL: "u1", Source: "GeekNews"},
		{Title: "H1", URL: "u2", Source: "HN"},
		{Title: "G2", URL: "u3", Source: "GeekNews"},
	}
	got := geekNewsItems(items)
	if len(got) != 2 || got[0].URL != "u1" || got[1].URL != "u3" {
		t.Fatalf("GeekNews만 순서대로 추려야 함: %+v", got)
	}
}

func TestBalanceNews_injectsWhenNonePresent(t *testing.T) {
	t.Parallel()
	items := []NewsItem{
		{Title: "긱뉴스글", URL: "https://hada.io/1", Desc: "긱설명", Source: "GeekNews"},
		{Title: "HN글", URL: "https://hn.com/2", Source: "HN"},
	}
	result := DigestResult{News: []NewsResult{{Title: "HN글", URL: "https://hn.com/2", Summary: "요약"}}}
	out := balanceNews(result, items)
	if len(out.News) != 2 {
		t.Fatalf("주입 후 2건 기대: %+v", out.News)
	}
	if out.News[0].URL != "https://hada.io/1" || out.News[0].Summary != "긱설명" {
		t.Fatalf("맨 앞에 GeekNews 주입 기대: %+v", out.News[0])
	}
}

func TestBalanceNews_capsExcessGeekNews(t *testing.T) {
	t.Parallel()
	// 후보·결과 모두 GeekNews 4개 + HN 1개. maxGeekNews=2 로 잘려 총 3건이 되어야 한다.
	items := []NewsItem{
		{URL: "g1", Source: "GeekNews"}, {URL: "g2", Source: "GeekNews"},
		{URL: "g3", Source: "GeekNews"}, {URL: "g4", Source: "GeekNews"},
		{URL: "h1", Source: "HN"},
	}
	result := DigestResult{News: []NewsResult{
		{URL: "g1"}, {URL: "g2"}, {URL: "g3"}, {URL: "g4"}, {URL: "h1"},
	}}
	out := balanceNews(result, items)
	if len(out.News) != 3 {
		t.Fatalf("GeekNews 2개 + HN 1개 = 3건 기대: %+v", out.News)
	}
	// 앞 2개 GeekNews(g1,g2)는 유지, g3·g4는 제거, HN은 유지.
	if out.News[0].URL != "g1" || out.News[1].URL != "g2" || out.News[2].URL != "h1" {
		t.Fatalf("초과 GeekNews 제거 실패: %+v", out.News)
	}
}

func TestBalanceNews_keepsMixUnchanged(t *testing.T) {
	t.Parallel()
	// 이미 GeekNews 1 + HN 2 로 균형 잡힌 경우 그대로 둔다.
	items := []NewsItem{
		{URL: "g1", Source: "GeekNews"}, {URL: "h1", Source: "HN"}, {URL: "h2", Source: "HN"},
	}
	result := DigestResult{News: []NewsResult{{URL: "g1"}, {URL: "h1"}, {URL: "h2"}}}
	out := balanceNews(result, items)
	if len(out.News) != 3 || out.News[0].URL != "g1" {
		t.Fatalf("균형 잡힌 결과는 그대로: %+v", out.News)
	}
}

func TestBalanceNews_emptyGeekNoop(t *testing.T) {
	t.Parallel()
	items := []NewsItem{{URL: "h1", Source: "HN"}}
	result := DigestResult{News: []NewsResult{{URL: "h1"}}}
	out := balanceNews(result, items)
	if len(out.News) != 1 {
		t.Fatalf("GeekNews 후보 없으면 그대로: %+v", out.News)
	}
}

func TestBuildTopicPrompt_specificDomain(t *testing.T) {
	t.Parallel()
	p := buildTopicPrompt("운영체제")
	if !strings.Contains(p, "운영체제") {
		t.Fatalf("지정 도메인 미포함: %q", p)
	}
	// 계층형 형식 안내가 있어야 한다.
	if !strings.Contains(p, "→") {
		t.Fatalf("계층형 형식 안내 없음: %q", p)
	}
	// 출력 JSON 스키마 안내(domain, topics).
	if !strings.Contains(p, "topics") || !strings.Contains(p, "domain") {
		t.Fatalf("JSON 스키마 안내 없음: %q", p)
	}
}

func TestBuildTopicPrompt_noHardcodedExample(t *testing.T) {
	t.Parallel()
	// 고정 예시("Vector DB → HNSW")가 매번 같은 주제를 유도하지 않도록 제거됐어야 한다.
	p := buildTopicPrompt("데이터베이스")
	if strings.Contains(p, "HNSW") || strings.Contains(p, "Vector DB") {
		t.Fatalf("고정 예시가 남아있음: %q", p)
	}
	// 전달된 도메인이 그대로 반영돼야 한다.
	if !strings.Contains(p, "데이터베이스") {
		t.Fatalf("도메인 미반영: %q", p)
	}
}

func TestChooseDomain_keepsRequested(t *testing.T) {
	t.Parallel()
	got := chooseDomain("운영체제", func(int) int { return 0 })
	if got != "운영체제" {
		t.Fatalf("지정 도메인 유지 실패: %q", got)
	}
}

func TestChooseDomain_randomWhenEmpty(t *testing.T) {
	t.Parallel()
	// randIntn 에는 도메인 개수가 전달되고, 그 인덱스의 도메인이 선택돼야 한다.
	got := chooseDomain("", func(n int) int {
		if n != len(domains) {
			t.Fatalf("randIntn 인자=%d, want %d", n, len(domains))
		}
		return 5
	})
	if got != domains[5] {
		t.Fatalf("랜덤 선택=%q, want %q", got, domains[5])
	}
}

// 컴파일 검증: Generator 인터페이스를 GeminiGenerator 가 구현하는지 확인한다.
var _ Generator = (*GeminiGenerator)(nil)
