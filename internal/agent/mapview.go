package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jongseong0111/jarvis/internal/notion"
)

// blockSink 은 MapRenderer 가 페이지를 다시 그릴 때 쓰는 Notion 블록 작업이다.
type blockSink interface {
	BlockChildren(ctx context.Context, blockID string) ([]string, error)
	DeleteBlock(ctx context.Context, blockID string) error
	AppendBlocks(ctx context.Context, blockID string, children []any) error
}

// MapRenderer 는 물건 데이터를 "우리집 지도" 페이지에 예쁘게 다시 그린다.
type MapRenderer struct {
	sink   blockSink
	pageID string
	home   HomePort
}

// NewMapRenderer 는 MapRenderer 를 생성한다.
func NewMapRenderer(sink blockSink, pageID string, home HomePort) *MapRenderer {
	return &MapRenderer{sink: sink, pageID: pageID, home: home}
}

// Render 는 현재 데이터로 지도 페이지를 통째로 다시 그린다.
func (r *MapRenderer) Render(ctx context.Context) error {
	items, err := r.home.Items(ctx)
	if err != nil {
		return err
	}
	locs, err := r.home.Locations(ctx)
	if err != nil {
		return err
	}
	blocks := buildMapBlocks(items, locs)

	ids, err := r.sink.BlockChildren(ctx, r.pageID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.sink.DeleteBlock(ctx, id); err != nil {
			return err
		}
	}
	return r.sink.AppendBlocks(ctx, r.pageID, blocks)
}

// zoneOrder 는 구역 표시 순서다(목록에 없는 구역은 뒤에 가나다순, "기타"는 맨 뒤).
var zoneOrder = []string{"거실", "거실복도", "거실 복도", "주방", "안방", "아기방", "로그방", "베란다", "창고", "욕실"}

var zoneEmoji = map[string]string{
	"거실": "🛋️", "거실복도": "🚪", "거실 복도": "🚪", "주방": "🍳", "안방": "🛏️", "아기방": "🧸",
	"로그방": "💻", "베란다": "🌿", "창고": "📦", "욕실": "🚿", "기타": "📍",
}

// buildMapBlocks 는 물건/장소로 지도 블록을 만든다. (순수 함수)
func buildMapBlocks(items []notion.Item, locs []notion.Location) []any {
	byID := indexLocations(locs)

	// zone -> locName -> []표시이름
	zones := map[string]map[string][]string{}
	for _, it := range items {
		loc := byID[it.LocationID]
		zone := it.Zone
		if zone == "" {
			zone = loc.Zone
		}
		if zone == "" {
			zone = "기타"
		}
		locName := loc.Name
		if locName == "" {
			locName = "(위치 미상)"
		}
		name := it.Name
		if it.Quantity != nil {
			name = fmt.Sprintf("%s(%d)", name, *it.Quantity)
		}
		if zones[zone] == nil {
			zones[zone] = map[string][]string{}
		}
		zones[zone][locName] = append(zones[zone][locName], name)
	}

	blocks := []any{calloutBlock("✨", "자비스가 자동으로 그려요. 직접 고치지 말고 @jarvis 한테 말하세요.", "gray_background")}
	if len(zones) == 0 {
		blocks = append(blocks, paragraphBlock("아직 등록된 물건이 없어요. @jarvis 한테 \"트롤리에 체온계 넣었어\" 라고 해보세요."))
		return blocks
	}

	for zi, zone := range orderedZones(zones) {
		emoji := zoneEmoji[zone]
		if emoji == "" {
			emoji = "📍"
		}
		if zi > 0 {
			blocks = append(blocks, dividerBlock())
		}
		blocks = append(blocks, headingBlock(emoji+" "+zone))
		color := zonePalette[zi%len(zonePalette)]
		locsInZone := zones[zone]
		for _, locName := range sortedKeys(locsInZone) {
			blocks = append(blocks, locationCallout(locName, strings.Join(locsInZone[locName], " · "), color))
		}
	}
	return blocks
}

// zonePalette 는 구역별로 돌아가며 쓰는 콜아웃 배경색이다.
var zonePalette = []string{
	"blue_background", "green_background", "orange_background", "purple_background",
	"pink_background", "yellow_background", "brown_background", "red_background",
}

// orderedZones 는 zoneOrder 우선, 나머지는 가나다순, "기타"는 맨 뒤로 정렬한다.
func orderedZones(zones map[string]map[string][]string) []string {
	rank := map[string]int{}
	for i, z := range zoneOrder {
		rank[z] = i
	}
	var out []string
	for z := range zones {
		out = append(out, z)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a == "기타" {
			return false
		}
		if b == "기타" {
			return true
		}
		ra, oka := rank[a]
		rb, okb := rank[b]
		switch {
		case oka && okb:
			return ra < rb
		case oka:
			return true
		case okb:
			return false
		default:
			return a < b
		}
	})
	return out
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- Notion 블록 빌더 ---

func headingBlock(text string) any {
	return map[string]any{
		"object": "block", "type": "heading_2",
		"heading_2": map[string]any{"rich_text": richText(text, false)},
	}
}

func paragraphBlock(text string) any {
	return map[string]any{
		"object": "block", "type": "paragraph",
		"paragraph": map[string]any{"rich_text": richText(text, false)},
	}
}

func calloutBlock(emoji, text, color string) any {
	return map[string]any{
		"object": "block", "type": "callout",
		"callout": map[string]any{
			"icon":      map[string]any{"type": "emoji", "emoji": emoji},
			"rich_text": richText(text, false),
			"color":     color,
		},
	}
}

// locationCallout 은 "📦 **장소**  item1 · item2" 카드형 콜아웃을 만든다.
func locationCallout(loc, items, color string) any {
	rt := []any{
		map[string]any{"type": "text", "text": map[string]any{"content": loc + "  "}, "annotations": map[string]any{"bold": true}},
		map[string]any{"type": "text", "text": map[string]any{"content": items}},
	}
	return map[string]any{
		"object": "block", "type": "callout",
		"callout": map[string]any{
			"icon":      map[string]any{"type": "emoji", "emoji": "📦"},
			"rich_text": rt,
			"color":     color,
		},
	}
}

func dividerBlock() any {
	return map[string]any{"object": "block", "type": "divider", "divider": map[string]any{}}
}

func richText(text string, bold bool) []any {
	return []any{map[string]any{
		"type":        "text",
		"text":        map[string]any{"content": text},
		"annotations": map[string]any{"bold": bold},
	}}
}
