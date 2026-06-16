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
	cats, err := r.home.Categories(ctx)
	if err != nil {
		return err
	}
	blocks := buildMapBlocks(items, locs, cats)

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

// catEmoji 는 카테고리 이름별 아이콘이다(없으면 빈 문자열).
var catEmoji = map[string]string{
	"화장품": "💄", "생활용품": "🧴", "청소용품": "🧹", "세탁용품": "🧺", "주방용품": "🍴",
	"서류": "📄", "문서": "📄", "의약품": "💊", "아기상비약": "💊", "성인상비약": "💊",
	"전자기기": "🔌", "충전기": "🔌", "케이블": "🔌", "배터리": "🔋",
	"공구": "🛠️", "의류": "👕", "육아용품": "🧸", "아기용품": "🧸", "아기위생": "🧼",
}

// buildMapBlocks 는 물건/장소/카테고리로 지도 블록을 만든다(장소→카테고리→물건). (순수 함수)
func buildMapBlocks(items []notion.Item, locs []notion.Location, cats []notion.Category) []any {
	locByID := indexLocations(locs)
	catByID := map[string]string{}
	for _, c := range cats {
		catByID[c.ID] = c.Name
	}

	// zone -> locName -> catName -> []표시이름
	zones := map[string]map[string]map[string][]string{}
	for _, it := range items {
		loc := locByID[it.LocationID]
		zone := firstNonEmpty(it.Zone, loc.Zone, "기타")
		locName := firstNonEmpty(loc.Name, "(위치 미상)")
		catName := firstNonEmpty(catByID[it.CategoryID], "기타")
		name := it.Name
		if it.Quantity != nil {
			name = fmt.Sprintf("%s(%d)", name, *it.Quantity)
		}
		if zones[zone] == nil {
			zones[zone] = map[string]map[string][]string{}
		}
		if zones[zone][locName] == nil {
			zones[zone][locName] = map[string][]string{}
		}
		zones[zone][locName][catName] = append(zones[zone][locName][catName], name)
	}

	blocks := []any{calloutBlock("✨", "자비스가 자동으로 그려요. 직접 고치지 말고 @jarvis 한테 말하세요.", "gray_background")}
	if len(zones) == 0 {
		blocks = append(blocks, paragraphBlock("아직 등록된 물건이 없어요. @jarvis 한테 \"트롤리에 체온계 넣었어\" 라고 해보세요."))
		return blocks
	}

	for zi, zone := range orderedZones(mapKeys(zones)) {
		emoji := firstNonEmpty(zoneEmoji[zone], "📍")
		if zi > 0 {
			blocks = append(blocks, dividerBlock())
		}
		blocks = append(blocks, headingBlock(emoji+" "+zone))
		color := zonePalette[zi%len(zonePalette)]
		for _, locName := range sortedStrings(mapKeys(zones[zone])) {
			children := []any{}
			for _, catName := range categoryOrder(mapKeys(zones[zone][locName])) {
				itemsStr := strings.Join(zones[zone][locName][catName], " · ")
				var line string
				if catName == "기타" {
					line = itemsStr
				} else {
					line = firstNonEmpty(catEmoji[catName]+" ", "") + catName + " — " + itemsStr
				}
				children = append(children, bulletBlock(line))
			}
			blocks = append(blocks, locationCallout(locName, color, children))
		}
	}
	return blocks
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sortedStrings(s []string) []string {
	sort.Strings(s)
	return s
}

// categoryOrder 는 카테고리를 가나다순 정렬하되 "기타"를 맨 뒤로 둔다.
func categoryOrder(keys []string) []string {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "기타" {
			return false
		}
		if keys[j] == "기타" {
			return true
		}
		return keys[i] < keys[j]
	})
	return keys
}

// zonePalette 는 구역별로 돌아가며 쓰는 콜아웃 배경색이다.
var zonePalette = []string{
	"blue_background", "green_background", "orange_background", "purple_background",
	"pink_background", "yellow_background", "brown_background", "red_background",
}

// orderedZones 는 zoneOrder 우선, 나머지는 가나다순, "기타"는 맨 뒤로 정렬한다.
func orderedZones(out []string) []string {
	rank := map[string]int{}
	for i, z := range zoneOrder {
		rank[z] = i
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

// locationCallout 은 "📦 **장소**" 카드 안에 카테고리별 항목(children)을 담은 콜아웃을 만든다.
func locationCallout(loc, color string, children []any) any {
	rt := []any{
		map[string]any{"type": "text", "text": map[string]any{"content": loc}, "annotations": map[string]any{"bold": true}},
	}
	callout := map[string]any{
		"icon":      map[string]any{"type": "emoji", "emoji": "📦"},
		"rich_text": rt,
		"color":     color,
	}
	if len(children) > 0 {
		callout["children"] = children
	}
	return map[string]any{"object": "block", "type": "callout", "callout": callout}
}

func bulletBlock(text string) any {
	return map[string]any{
		"object": "block", "type": "bulleted_list_item",
		"bulleted_list_item": map[string]any{"rich_text": richText(text, false)},
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
