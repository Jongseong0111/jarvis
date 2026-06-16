// Package agent 는 도구(tool)를 가진 LLM 에이전트로 메시지를 처리한다.
package agent

import (
	"context"

	"github.com/Jongseong0111/jarvis/internal/notion"
)

// HomePort 는 집정리 도구가 필요로 하는 Notion 작업이다(테스트에서 fake 주입).
type HomePort interface {
	Locations(ctx context.Context) ([]notion.Location, error)
	Categories(ctx context.Context) ([]notion.Category, error)
	Items(ctx context.Context) ([]notion.Item, error)
	SearchItems(ctx context.Context, name string) ([]notion.Item, error)
	CreateItem(ctx context.Context, name, categoryID, locationID, zone string, quantity *int) (string, error)
	CreateLocation(ctx context.Context, name, zone string) (string, error)
	UpdateItem(ctx context.Context, itemID, locationID, zone string, quantity *int) error
	ArchiveItem(ctx context.Context, itemID string) error
	ArchiveLocation(ctx context.Context, locationID string) error
}

// NotionHome 은 *notion.Client 를 HomePort 로 감싼다(3 DB ID 보유).
type NotionHome struct {
	client       *notion.Client
	locationsDB  string
	categoriesDB string
	itemsDB      string
}

// NewNotionHome 은 NotionHome 을 생성한다.
func NewNotionHome(client *notion.Client, locationsDB, categoriesDB, itemsDB string) NotionHome {
	return NotionHome{client: client, locationsDB: locationsDB, categoriesDB: categoriesDB, itemsDB: itemsDB}
}

// Locations 는 장소 DB 전체를 조회한다.
func (h NotionHome) Locations(ctx context.Context) ([]notion.Location, error) {
	pages, err := h.client.QueryDatabase(ctx, h.locationsDB, nil)
	if err != nil {
		return nil, err
	}
	out := make([]notion.Location, len(pages))
	for i, p := range pages {
		out[i] = notion.ParseLocation(p)
	}
	return out, nil
}

// Categories 는 카테고리 DB 전체를 조회한다.
func (h NotionHome) Categories(ctx context.Context) ([]notion.Category, error) {
	pages, err := h.client.QueryDatabase(ctx, h.categoriesDB, nil)
	if err != nil {
		return nil, err
	}
	out := make([]notion.Category, len(pages))
	for i, p := range pages {
		out[i] = notion.ParseCategory(p)
	}
	return out, nil
}

// Items 는 물건 DB 전체를 조회한다.
func (h NotionHome) Items(ctx context.Context) ([]notion.Item, error) {
	pages, err := h.client.QueryDatabase(ctx, h.itemsDB, nil)
	if err != nil {
		return nil, err
	}
	out := make([]notion.Item, len(pages))
	for i, p := range pages {
		out[i] = notion.ParseItem(p)
	}
	return out, nil
}

// SearchItems 는 이름에 name 을 포함하는 물건을 조회한다.
func (h NotionHome) SearchItems(ctx context.Context, name string) ([]notion.Item, error) {
	pages, err := h.client.QueryDatabase(ctx, h.itemsDB, notion.TitleContainsFilter(notion.PropName, name))
	if err != nil {
		return nil, err
	}
	out := make([]notion.Item, len(pages))
	for i, p := range pages {
		out[i] = notion.ParseItem(p)
	}
	return out, nil
}

// CreateItem 은 물건 DB 에 page 를 생성한다.
func (h NotionHome) CreateItem(ctx context.Context, name, categoryID, locationID, zone string, quantity *int) (string, error) {
	return h.client.CreatePage(ctx, h.itemsDB, notion.ItemProperties(name, categoryID, locationID, zone, quantity))
}

// CreateLocation 은 장소 DB 에 page 를 생성한다.
func (h NotionHome) CreateLocation(ctx context.Context, name, zone string) (string, error) {
	return h.client.CreatePage(ctx, h.locationsDB, notion.LocationProperties(name, zone))
}

// UpdateItem 은 물건의 위치/구역/수량을 갱신한다(빈 값은 변경 안 함).
func (h NotionHome) UpdateItem(ctx context.Context, itemID, locationID, zone string, quantity *int) error {
	return h.client.UpdatePage(ctx, itemID, notion.ItemUpdateProperties(locationID, zone, quantity))
}

// ArchiveItem 은 물건을 삭제(보관)한다.
func (h NotionHome) ArchiveItem(ctx context.Context, itemID string) error {
	return h.client.ArchivePage(ctx, itemID)
}

// ArchiveLocation 은 장소를 삭제(보관)한다.
func (h NotionHome) ArchiveLocation(ctx context.Context, locationID string) error {
	return h.client.ArchivePage(ctx, locationID)
}
