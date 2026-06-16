package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/internal/notion"
)

func TestBuildMapBlocks(t *testing.T) {
	t.Parallel()
	locs := []notion.Location{
		{ID: "l1", Name: "아기 트롤리", Zone: "거실"},
		{ID: "l2", Name: "베란다 수납장2", Zone: "베란다"},
	}
	items := []notion.Item{
		{Name: "체온계", LocationID: "l1", Zone: "거실"},
		{Name: "가위", LocationID: "l1", Zone: "거실"},
		{Name: "세제", LocationID: "l2", Zone: "베란다"},
	}
	blocks := buildMapBlocks(items, locs)
	b, _ := json.Marshal(blocks)
	s := string(b)

	// 콜아웃 + 거실/베란다 heading + 항목들
	for _, want := range []string{"거실", "베란다", "아기 트롤리", "체온계", "가위", "세제", "heading_2"} {
		if !strings.Contains(s, want) {
			t.Fatalf("지도 블록에 %q 없음", want)
		}
	}
	// 거실이 베란다보다 먼저(zoneOrder)
	if strings.Index(s, "거실") > strings.Index(s, "베란다") {
		t.Fatal("구역 순서가 zoneOrder 를 안 따름")
	}
}

func TestBuildMapBlocks_empty(t *testing.T) {
	t.Parallel()
	blocks := buildMapBlocks(nil, nil)
	b, _ := json.Marshal(blocks)
	if !strings.Contains(string(b), "아직 등록된 물건이 없어요") {
		t.Fatalf("빈 상태 안내가 없음: %s", string(b))
	}
}
