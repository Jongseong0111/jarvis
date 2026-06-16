// Package workers 는 intent 별 작업을 수행하는 Worker 들을 구현한다.
package workers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/notion"
)

// NotionPort 는 HomeWorker 가 필요로 하는 Notion 작업이다(테스트에서 fake 주입).
type NotionPort interface {
	Locations(ctx context.Context) ([]notion.Location, error)
	Categories(ctx context.Context) ([]notion.Category, error)
	SearchItems(ctx context.Context, name string) ([]notion.Item, error)
	CreateItem(ctx context.Context, name, categoryID, locationID, zone string, quantity *int) (string, error)
}

// Home 은 집 정리(home.*) intent 를 처리한다. domain.Worker + domain.ProposalApplier 를 구현한다.
type Home struct {
	notion    NotionPort
	extractor Extractor
}

// NewHome 은 Home Worker 를 생성한다.
func NewHome(port NotionPort, extractor Extractor) Home {
	return Home{notion: port, extractor: extractor}
}

// Handle 은 home intent 를 처리한다(검색=즉시 응답, 추가=변경안+버튼).
func (w Home) Handle(ctx context.Context, intent domain.Intent, in domain.IncomingMessage) (domain.Reply, error) {
	switch intent {
	case domain.IntentHomeSearch, domain.IntentHomeAdd:
		// 아래에서 처리
	default: // home.update / home.delete
		return textReply(in.ChannelID, "그 작업(수정/삭제)은 아직 준비 중이야. 곧 추가할게."), nil
	}

	locations, err := w.notion.Locations(ctx)
	if err != nil {
		return domain.Reply{}, fmt.Errorf("장소 로드 실패: %w", err)
	}
	categories, err := w.notion.Categories(ctx)
	if err != nil {
		return domain.Reply{}, fmt.Errorf("카테고리 로드 실패: %w", err)
	}

	ex, err := w.extractor.Extract(ctx, in.Text, locationNames(locations), categoryNames(categories))
	if err != nil {
		return domain.Reply{}, fmt.Errorf("집정리 추출 실패: %w", err)
	}
	if strings.TrimSpace(ex.Item) == "" {
		return textReply(in.ChannelID, "어떤 물건인지 잘 모르겠어. 물건 이름을 알려줄래?"), nil
	}

	if intent == domain.IntentHomeSearch {
		return w.handleSearch(ctx, in.ChannelID, ex, locations, categories)
	}
	return w.handleAdd(in.ChannelID, ex, locations, categories)
}

func (w Home) handleSearch(ctx context.Context, channelID string, ex Extracted, locations []notion.Location, categories []notion.Category) (domain.Reply, error) {
	items, err := w.notion.SearchItems(ctx, ex.Item)
	if err != nil {
		return domain.Reply{}, fmt.Errorf("물건 검색 실패: %w", err)
	}
	locByID := indexLocations(locations)

	if len(items) > 0 {
		var lines []string
		for _, it := range items {
			loc := locByID[it.LocationID]
			lines = append(lines, formatItemLocation(it, loc))
		}
		return textReply(channelID, strings.Join(lines, "\n")), nil
	}

	// 못 찾았으면 카테고리 기본장소 제안
	if cat := findCategory(categories, ex.Category); cat != nil && cat.DefaultLocationID != "" {
		loc := locByID[cat.DefaultLocationID]
		return textReply(channelID, fmt.Sprintf("'%s'은(는) 아직 등록 안 했어. %s 쪽에 두면 될 것 같아 (%s 기본 위치).", ex.Item, loc.Name, cat.Name)), nil
	}
	return textReply(channelID, fmt.Sprintf("'%s'을(를) 못 찾았어.", ex.Item)), nil
}

func (w Home) handleAdd(channelID string, ex Extracted, locations []notion.Location, categories []notion.Category) (domain.Reply, error) {
	loc := findLocation(locations, ex.Location)
	if loc == nil {
		return textReply(channelID, fmt.Sprintf("'%s'을(를) 못 찾았어. 등록된 장소 중에서 알려줄래?\n(%s)",
			ex.Location, strings.Join(locationNames(locations), ", "))), nil
	}
	p := domain.ChangeProposal{
		Action:       "add",
		ItemName:     ex.Item,
		LocationID:   loc.ID,
		LocationName: loc.Name,
		LocationZone: loc.Zone,
		Quantity:     ex.Quantity,
	}
	if cat := findCategory(categories, ex.Category); cat != nil {
		p.CategoryID = cat.ID
		p.CategoryName = cat.Name
	}
	return proposalReply(channelID, p), nil
}

// Apply 는 승인된 변경안을 Notion 에 반영한다. ChannelID 는 호출자가 채운다.
func (w Home) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if p.ItemName == "" || p.LocationID == "" {
		return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item/location 누락)")
	}
	if _, err := w.notion.CreateItem(ctx, p.ItemName, p.CategoryID, p.LocationID, p.LocationZone, p.Quantity); err != nil {
		return domain.Reply{}, fmt.Errorf("물건 추가 실패: %w", err)
	}
	return domain.Reply{Text: fmt.Sprintf("✅ '%s'을(를) %s에 추가했어.", p.ItemName, p.LocationName)}, nil
}

// --- 순수 helper ---

func textReply(channelID, text string) domain.Reply {
	return domain.Reply{ChannelID: channelID, Text: text}
}

// proposalReply 는 변경안을 요약 + 승인/취소 버튼이 달린 Reply 로 만든다.
func proposalReply(channelID string, p domain.ChangeProposal) domain.Reply {
	return domain.Reply{
		ChannelID: channelID,
		Text:      formatProposal(p),
		Buttons: []domain.Button{
			{Text: "승인", Action: "approve", Value: p.Encode(), Style: "primary"},
			{Text: "취소", Action: "cancel"},
		},
	}
}

func formatProposal(p domain.ChangeProposal) string {
	var b strings.Builder
	b.WriteString("변경안: 물건 추가\n")
	fmt.Fprintf(&b, "품목: %s\n", p.ItemName)
	fmt.Fprintf(&b, "위치: %s\n", p.LocationName)
	if p.CategoryName != "" {
		fmt.Fprintf(&b, "카테고리: %s\n", p.CategoryName)
	}
	if p.Quantity != nil {
		fmt.Fprintf(&b, "수량: %d\n", *p.Quantity)
	}
	b.WriteString("\n적용할까?")
	return b.String()
}

func formatItemLocation(it notion.Item, loc notion.Location) string {
	where := loc.Name
	if loc.Zone != "" {
		where = fmt.Sprintf("%s (%s)", loc.Name, loc.Zone)
	}
	if where == "" {
		where = "위치 미상"
	}
	line := fmt.Sprintf("%s: %s", it.Name, where)
	if it.Quantity != nil {
		line = fmt.Sprintf("%s, %d개", line, *it.Quantity)
	}
	return line
}

func locationNames(locs []notion.Location) []string {
	out := make([]string, len(locs))
	for i, l := range locs {
		out[i] = l.Name
	}
	return out
}

func categoryNames(cats []notion.Category) []string {
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = c.Name
	}
	return out
}

func indexLocations(locs []notion.Location) map[string]notion.Location {
	m := make(map[string]notion.Location, len(locs))
	for _, l := range locs {
		m[l.ID] = l
	}
	return m
}

func findLocation(locs []notion.Location, name string) *notion.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for i := range locs {
		if strings.EqualFold(locs[i].Name, name) {
			return &locs[i]
		}
	}
	return nil
}

func findCategory(cats []notion.Category, name string) *notion.Category {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for i := range cats {
		if strings.EqualFold(cats[i].Name, name) {
			return &cats[i]
		}
	}
	return nil
}
