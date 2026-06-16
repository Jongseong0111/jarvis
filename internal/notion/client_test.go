package notion

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New("test-key")
	c.baseURL = srv.URL
	return c
}

func TestClient_QueryDatabase(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization 헤더 = %q", got)
		}
		if got := r.Header.Get("Notion-Version"); got != notionVersion {
			t.Errorf("Notion-Version 헤더 = %q", got)
		}
		_, _ = io.WriteString(w, `{
			"results": [
				{"id": "loc-1", "properties": {
					"이름": {"type":"title","title":[{"plain_text":"아기 트롤리"}]},
					"구역": {"type":"select","select":{"name":"거실"}}
				}}
			],
			"has_more": false
		}`)
	}))
	defer srv.Close()

	pages, err := newTestClient(srv).QueryDatabase(context.Background(), "db-1", nil)
	if err != nil {
		t.Fatalf("QueryDatabase: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("pages 수 = %d, want 1", len(pages))
	}
	loc := ParseLocation(pages[0])
	if loc.ID != "loc-1" || loc.Name != "아기 트롤리" || loc.Zone != "거실" {
		t.Fatalf("ParseLocation = %+v", loc)
	}
}

func TestClient_CreatePage(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = io.WriteString(w, `{"id":"new-page-1"}`)
	}))
	defer srv.Close()

	qty := 4
	props := ItemProperties("AAA 건전지", "cat-1", "loc-1", "로그방", &qty)
	id, err := newTestClient(srv).CreatePage(context.Background(), "items-db", props)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if id != "new-page-1" {
		t.Fatalf("생성 ID = %q, want new-page-1", id)
	}
	parent, _ := gotBody["parent"].(map[string]any)
	if parent["database_id"] != "items-db" {
		t.Fatalf("parent.database_id = %v", parent["database_id"])
	}
	if _, ok := gotBody["properties"].(map[string]any)["수량"]; !ok {
		t.Fatalf("properties 에 수량이 없음: %v", gotBody["properties"])
	}
}

func TestItemProperties_optional(t *testing.T) {
	t.Parallel()
	// 카테고리/구역/수량 없으면 해당 키 미포함
	props := ItemProperties("체온계", "", "loc-2", "", nil)
	if _, ok := props[PropCategory]; ok {
		t.Fatal("categoryID 비었는데 카테고리 속성이 포함됨")
	}
	if _, ok := props[PropZone]; ok {
		t.Fatal("zone 비었는데 구역 속성이 포함됨")
	}
	if _, ok := props[PropQuantity]; ok {
		t.Fatal("quantity nil 인데 수량 속성이 포함됨")
	}
	if _, ok := props[PropLocation]; !ok {
		t.Fatal("현재위치는 항상 포함되어야 함")
	}
}

func TestParseItem_relationsAndNumber(t *testing.T) {
	t.Parallel()
	p := Page{ID: "item-1", Properties: map[string]Property{
		PropName:     {Type: "title", Title: []RichText{{PlainText: "AAA 건전지"}}},
		PropCategory: {Type: "relation", Relation: []RelationRef{{ID: "cat-1"}}},
		PropLocation: {Type: "relation", Relation: []RelationRef{{ID: "loc-1"}}},
		PropQuantity: {Type: "number", Number: ptrFloat(4)},
	}}
	it := ParseItem(p)
	if it.Name != "AAA 건전지" || it.CategoryID != "cat-1" || it.LocationID != "loc-1" {
		t.Fatalf("ParseItem = %+v", it)
	}
	if it.Quantity == nil || *it.Quantity != 4 {
		t.Fatalf("Quantity = %v, want 4", it.Quantity)
	}
}

func ptrFloat(f float64) *float64 { return &f }
