package agent

import (
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/notion"
)

func TestFindLocation_spaceInsensitive(t *testing.T) {
	t.Parallel()
	// "로그방 팬트리"(공백 다름)로 찾아도 "로그 방 팬트리"에 매칭돼야 한다.
	locs := []notion.Location{{ID: "1", Name: "로그 방 팬트리", Zone: "로그 방"}}
	got, err := findLocation(locs, "로그방 팬트리", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.ID != "1" {
		t.Fatalf("공백 무시 매칭 실패: %+v", got)
	}
}

func TestFindLocation_ambiguousWithoutZone(t *testing.T) {
	t.Parallel()
	// 같은 이름 "팬트리"가 두 구역에 있고 zone 이 없으면 silently 첫 매칭이 아니라 에러(되묻기).
	locs := []notion.Location{
		{ID: "1", Name: "팬트리", Zone: "거실 복도"},
		{ID: "2", Name: "팬트리", Zone: "로그 방"},
	}
	got, err := findLocation(locs, "팬트리", "")
	if err == nil {
		t.Fatal("동명 다구역 + zone 미지정이면 에러를 기대")
	}
	if got != nil {
		t.Fatalf("모호 시 nil 기대: %+v", got)
	}
}

func TestFindLocation_disambiguatesByZone(t *testing.T) {
	t.Parallel()
	locs := []notion.Location{
		{ID: "1", Name: "팬트리", Zone: "거실 복도"},
		{ID: "2", Name: "팬트리", Zone: "로그 방"},
	}
	got, err := findLocation(locs, "팬트리", "로그방") // zone 도 공백 무시
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.ID != "2" {
		t.Fatalf("로그 방 팬트리를 기대: %+v", got)
	}
}

func TestFindLocation_notFound(t *testing.T) {
	t.Parallel()
	got, err := findLocation([]notion.Location{{Name: "책장", Zone: "안방"}}, "팬트리", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("못 찾으면 nil 기대: %+v", got)
	}
}

func TestFormatLocationsHint(t *testing.T) {
	t.Parallel()
	locs := []notion.Location{
		{Name: "팬트리", Zone: "거실 복도"},
		{Name: "팬트리", Zone: "로그 방"},
		{Name: "책장", Zone: "로그 방"},
	}
	s := formatLocationsHint(locs)
	if !strings.Contains(s, "거실 복도") || !strings.Contains(s, "로그 방") {
		t.Fatalf("구역 누락: %q", s)
	}
	if !strings.Contains(s, "팬트리") || !strings.Contains(s, "책장") {
		t.Fatalf("장소 누락: %q", s)
	}
	if !strings.Contains(s, "zone") {
		t.Fatalf("zone 사용 안내 누락: %q", s)
	}
}
