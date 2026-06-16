package workers

import (
	"context"
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/notion"
)

type createdItem struct {
	name, categoryID, locationID, zone string
	quantity                           *int
}

type fakeNotion struct {
	locations  []notion.Location
	categories []notion.Category
	items      []notion.Item
	created    *createdItem
	createErr  error
}

func (f *fakeNotion) Locations(context.Context) ([]notion.Location, error)  { return f.locations, nil }
func (f *fakeNotion) Categories(context.Context) ([]notion.Category, error) { return f.categories, nil }
func (f *fakeNotion) SearchItems(context.Context, string) ([]notion.Item, error) {
	return f.items, nil
}
func (f *fakeNotion) CreateItem(_ context.Context, name, categoryID, locationID, zone string, quantity *int) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = &createdItem{name, categoryID, locationID, zone, quantity}
	return "new-id", nil
}

type fakeExtractor struct {
	result Extracted
}

func (f fakeExtractor) Extract(context.Context, string, []string, []string) (Extracted, error) {
	return f.result, nil
}

func newHome(n *fakeNotion, ex Extracted) Home {
	return NewHome(n, fakeExtractor{result: ex})
}

var (
	trolley = notion.Location{ID: "loc-1", Name: "아기 트롤리", Zone: "거실"}
	veranda = notion.Location{ID: "loc-2", Name: "베란다 수납장2", Zone: "베란다"}
	babyMed = notion.Category{ID: "cat-1", Name: "아기상비약"}
	laundry = notion.Category{ID: "cat-2", Name: "세탁용품", DefaultLocationID: "loc-2"}
)

func TestHome_Search_found(t *testing.T) {
	t.Parallel()
	n := &fakeNotion{
		locations: []notion.Location{trolley},
		items:     []notion.Item{{Name: "체온계", LocationID: "loc-1"}},
	}
	h := newHome(n, Extracted{Action: "search", Item: "체온계"})
	reply, err := h.Handle(context.Background(), domain.IntentHomeSearch, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(reply.Buttons) != 0 {
		t.Fatal("검색 응답엔 버튼이 없어야 함")
	}
	for _, want := range []string{"체온계", "아기 트롤리", "거실"} {
		if !strings.Contains(reply.Text, want) {
			t.Fatalf("응답에 %q 없음: %q", want, reply.Text)
		}
	}
}

func TestHome_Search_suggestDefaultLocation(t *testing.T) {
	t.Parallel()
	n := &fakeNotion{
		locations:  []notion.Location{veranda},
		categories: []notion.Category{laundry},
		items:      nil, // 못 찾음
	}
	h := newHome(n, Extracted{Action: "search", Item: "세제", Category: "세탁용품"})
	reply, err := h.Handle(context.Background(), domain.IntentHomeSearch, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.Contains(reply.Text, "베란다 수납장2") {
		t.Fatalf("기본장소 제안이 없음: %q", reply.Text)
	}
}

func TestHome_Add_proposal(t *testing.T) {
	t.Parallel()
	n := &fakeNotion{
		locations:  []notion.Location{trolley},
		categories: []notion.Category{babyMed},
	}
	h := newHome(n, Extracted{Action: "add", Item: "체온계", Location: "아기 트롤리", Category: "아기상비약"})
	reply, err := h.Handle(context.Background(), domain.IntentHomeAdd, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(reply.Buttons) != 2 || reply.Buttons[0].Action != "approve" || reply.Buttons[1].Action != "cancel" {
		t.Fatalf("승인/취소 버튼이 없음: %+v", reply.Buttons)
	}
	p, err := domain.DecodeProposal(reply.Buttons[0].Value)
	if err != nil {
		t.Fatalf("DecodeProposal: %v", err)
	}
	if p.ItemName != "체온계" || p.LocationID != "loc-1" || p.CategoryID != "cat-1" || p.LocationZone != "거실" {
		t.Fatalf("변경안 = %+v", p)
	}
}

func TestHome_Add_locationNotFound(t *testing.T) {
	t.Parallel()
	n := &fakeNotion{locations: []notion.Location{trolley}}
	h := newHome(n, Extracted{Action: "add", Item: "체온계", Location: "없는장소"})
	reply, err := h.Handle(context.Background(), domain.IntentHomeAdd, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(reply.Buttons) != 0 {
		t.Fatal("resolve 실패 시 버튼이 없어야 함")
	}
	if !strings.Contains(reply.Text, "못 찾았어") {
		t.Fatalf("되묻기 메시지가 아님: %q", reply.Text)
	}
}

func TestHome_Apply_createsItem(t *testing.T) {
	t.Parallel()
	n := &fakeNotion{}
	h := NewHome(n, fakeExtractor{})
	qty := 4
	reply, err := h.Apply(context.Background(), domain.ChangeProposal{
		Action: "add", ItemName: "AAA 건전지", CategoryID: "cat-1", LocationID: "loc-1", LocationName: "로그방 서랍", LocationZone: "로그방", Quantity: &qty,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if n.created == nil || n.created.name != "AAA 건전지" || n.created.locationID != "loc-1" || n.created.categoryID != "cat-1" || n.created.zone != "로그방" {
		t.Fatalf("CreateItem 인자 = %+v", n.created)
	}
	if n.created.quantity == nil || *n.created.quantity != 4 {
		t.Fatalf("수량 = %v", n.created.quantity)
	}
	if !strings.Contains(reply.Text, "추가했어") {
		t.Fatalf("확인 메시지가 아님: %q", reply.Text)
	}
}

func TestHome_updateDeferred(t *testing.T) {
	t.Parallel()
	h := NewHome(&fakeNotion{}, fakeExtractor{})
	reply, err := h.Handle(context.Background(), domain.IntentHomeUpdate, domain.IncomingMessage{ChannelID: "C1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.Contains(reply.Text, "준비 중") {
		t.Fatalf("준비 중 안내가 아님: %q", reply.Text)
	}
}
