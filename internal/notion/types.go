// Package notion 은 Notion REST API 의 thin 클라이언트다(우리 3 DB 에 필요한 것만).
package notion

// Notion property 이름(스키마 의존). 스키마 변경 시 여기만 고치면 된다.
const (
	PropName            = "이름"
	PropZone            = "구역"
	PropType            = "타입"
	PropParentLocation  = "상위장소"
	PropParentCategory  = "상위카테고리"
	PropDefaultLocation = "기본장소"
	PropCategory        = "카테고리"
	PropLocation        = "현재위치"
	PropQuantity        = "수량"
	PropMemo            = "메모"
)

// --- Notion 응답 파싱용 구조 ---

// Page 는 Notion page 객체(필요한 필드만)다.
type Page struct {
	ID         string              `json:"id"`
	Properties map[string]Property `json:"properties"`
}

// Property 는 Notion page property(여러 타입의 합집합)다.
type Property struct {
	Type     string        `json:"type"`
	Title    []RichText    `json:"title"`
	RichText []RichText    `json:"rich_text"`
	Select   *SelectOption `json:"select"`
	Number   *float64      `json:"number"`
	Relation []RelationRef `json:"relation"`
}

// RichText 는 title/rich_text 요소다.
type RichText struct {
	PlainText string `json:"plain_text"`
}

// SelectOption 은 select property 값이다.
type SelectOption struct {
	Name string `json:"name"`
}

// RelationRef 는 relation property 의 연결 대상 page ID 다.
type RelationRef struct {
	ID string `json:"id"`
}

// --- 도메인 DTO (relation 은 ID 만 — 이름은 호출자가 마스터데이터로 resolve) ---

// Location 은 장소 DB 의 한 행이다.
type Location struct {
	ID   string
	Name string
	Zone string // 구역(select)
}

// Category 는 카테고리 DB 의 한 행이다.
type Category struct {
	ID                string
	Name              string
	DefaultLocationID string // 기본장소(relation) 첫 ID
}

// Item 은 물건 DB 의 한 행이다.
type Item struct {
	ID         string
	Name       string
	CategoryID string // 카테고리(relation) 첫 ID
	LocationID string // 현재위치(relation) 첫 ID
	Zone       string // 구역(select, 비정규화)
	Quantity   *int
}

// --- Page → DTO 파서 ---

// ParseLocation 은 Page 를 Location 으로 변환한다.
func ParseLocation(p Page) Location {
	return Location{ID: p.ID, Name: titleOf(p, PropName), Zone: selectOf(p, PropZone)}
}

// ParseCategory 는 Page 를 Category 로 변환한다.
func ParseCategory(p Page) Category {
	return Category{ID: p.ID, Name: titleOf(p, PropName), DefaultLocationID: firstRelationOf(p, PropDefaultLocation)}
}

// ParseItem 은 Page 를 Item 으로 변환한다.
func ParseItem(p Page) Item {
	return Item{
		ID:         p.ID,
		Name:       titleOf(p, PropName),
		CategoryID: firstRelationOf(p, PropCategory),
		LocationID: firstRelationOf(p, PropLocation),
		Zone:       selectOf(p, PropZone),
		Quantity:   numberOf(p, PropQuantity),
	}
}

func titleOf(p Page, key string) string {
	if v, ok := p.Properties[key]; ok && len(v.Title) > 0 {
		return v.Title[0].PlainText
	}
	return ""
}

func selectOf(p Page, key string) string {
	if v, ok := p.Properties[key]; ok && v.Select != nil {
		return v.Select.Name
	}
	return ""
}

func firstRelationOf(p Page, key string) string {
	if v, ok := p.Properties[key]; ok && len(v.Relation) > 0 {
		return v.Relation[0].ID
	}
	return ""
}

func numberOf(p Page, key string) *int {
	if v, ok := p.Properties[key]; ok && v.Number != nil {
		n := int(*v.Number)
		return &n
	}
	return nil
}

// --- CreatePage property 빌더 ---

// ItemProperties 는 Items DB page 생성을 위한 properties 맵을 만든다.
// categoryID/zone/quantity 가 비면 해당 속성을 넣지 않는다.
// zone(구역 select)은 위치에서 끌어와 대시보드 그룹용으로 비정규화 저장한다.
func ItemProperties(name, categoryID, locationID, zone string, quantity *int) map[string]any {
	props := map[string]any{
		PropName:     titleProp(name),
		PropLocation: relationProp(locationID),
	}
	if categoryID != "" {
		props[PropCategory] = relationProp(categoryID)
	}
	if zone != "" {
		props[PropZone] = map[string]any{"select": map[string]any{"name": zone}}
	}
	if quantity != nil {
		props[PropQuantity] = map[string]any{"number": *quantity}
	}
	return props
}

// ItemUpdateProperties 는 물건 수정용 부분 properties 맵이다. 빈 값/ nil 은 포함하지 않는다.
func ItemUpdateProperties(categoryID, locationID, zone string, quantity *int) map[string]any {
	props := map[string]any{}
	if categoryID != "" {
		props[PropCategory] = relationProp(categoryID)
	}
	if locationID != "" {
		props[PropLocation] = relationProp(locationID)
	}
	if zone != "" {
		props[PropZone] = map[string]any{"select": map[string]any{"name": zone}}
	}
	if quantity != nil {
		props[PropQuantity] = map[string]any{"number": *quantity}
	}
	return props
}

// CategoryProperties 는 Categories DB page 생성을 위한 properties 맵을 만든다.
func CategoryProperties(name string) map[string]any {
	return map[string]any{PropName: titleProp(name)}
}

// LocationProperties 는 Locations DB page 생성을 위한 properties 맵을 만든다.
// zone(구역 select)이 비면 넣지 않는다. 타입은 Storage 로 기본 지정한다(자리=수납).
func LocationProperties(name, zone string) map[string]any {
	props := map[string]any{
		PropName: titleProp(name),
		PropType: map[string]any{"select": map[string]any{"name": "Storage"}},
	}
	if zone != "" {
		props[PropZone] = map[string]any{"select": map[string]any{"name": zone}}
	}
	return props
}

// LocationUpdateProperties 는 장소 수정용 부분 properties 맵이다(이름/구역만, 빈 값은 제외).
// 생성과 달리 Type 은 건드리지 않는다.
func LocationUpdateProperties(name, zone string) map[string]any {
	props := map[string]any{}
	if name != "" {
		props[PropName] = titleProp(name)
	}
	if zone != "" {
		props[PropZone] = map[string]any{"select": map[string]any{"name": zone}}
	}
	return props
}

func titleProp(s string) map[string]any {
	return map[string]any{"title": []any{map[string]any{"text": map[string]any{"content": s}}}}
}

func relationProp(id string) map[string]any {
	return map[string]any{"relation": []any{map[string]any{"id": id}}}
}

// TitleContainsFilter 는 title 속성 contains 필터 바디를 만든다.
func TitleContainsFilter(prop, value string) map[string]any {
	return map[string]any{
		"filter": map[string]any{
			"property": prop,
			"title":    map[string]any{"contains": value},
		},
	}
}
