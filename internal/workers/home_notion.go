package workers

import (
	"context"

	"github.com/Jongseong0111/jarvis/internal/notion"
)

// NotionAdapter 는 *notion.Client 를 HomeWorker 의 NotionPort 로 감싼다(3 DB ID 보유).
type NotionAdapter struct {
	client       *notion.Client
	locationsDB  string
	categoriesDB string
	itemsDB      string
}

// NewNotionAdapter 는 NotionAdapter 를 생성한다.
func NewNotionAdapter(client *notion.Client, locationsDB, categoriesDB, itemsDB string) NotionAdapter {
	return NotionAdapter{client: client, locationsDB: locationsDB, categoriesDB: categoriesDB, itemsDB: itemsDB}
}

// Locations 는 장소 DB 전체를 조회한다.
func (a NotionAdapter) Locations(ctx context.Context) ([]notion.Location, error) {
	pages, err := a.client.QueryDatabase(ctx, a.locationsDB, nil)
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
func (a NotionAdapter) Categories(ctx context.Context) ([]notion.Category, error) {
	pages, err := a.client.QueryDatabase(ctx, a.categoriesDB, nil)
	if err != nil {
		return nil, err
	}
	out := make([]notion.Category, len(pages))
	for i, p := range pages {
		out[i] = notion.ParseCategory(p)
	}
	return out, nil
}

// SearchItems 는 이름에 name 을 포함하는 물건을 조회한다.
func (a NotionAdapter) SearchItems(ctx context.Context, name string) ([]notion.Item, error) {
	pages, err := a.client.QueryDatabase(ctx, a.itemsDB, notion.TitleContainsFilter(notion.PropName, name))
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
func (a NotionAdapter) CreateItem(ctx context.Context, name, categoryID, locationID, zone string, quantity *int) (string, error) {
	return a.client.CreatePage(ctx, a.itemsDB, notion.ItemProperties(name, categoryID, locationID, zone, quantity))
}
