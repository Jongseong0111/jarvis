package domain

import (
	"encoding/json"
	"testing"
)

func TestChangeProposal_JSONRoundtrip(t *testing.T) {
	t.Parallel()
	qty := 4
	tests := []struct {
		name string
		in   ChangeProposal
	}{
		{
			name: "수량 있음",
			in: ChangeProposal{
				Action: "add", ItemName: "AAA 건전지",
				CategoryID: "cat-1", CategoryName: "전자기기",
				LocationID: "loc-1", LocationName: "로그방 서랍",
				Quantity: &qty,
			},
		},
		{
			name: "수량/카테고리 없음",
			in: ChangeProposal{
				Action: "add", ItemName: "체온계",
				LocationID: "loc-2", LocationName: "아기 트롤리",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got ChangeProposal
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Action != tt.in.Action || got.ItemName != tt.in.ItemName ||
				got.LocationID != tt.in.LocationID || got.LocationName != tt.in.LocationName ||
				got.CategoryID != tt.in.CategoryID || got.CategoryName != tt.in.CategoryName {
				t.Fatalf("문자열 필드 불일치: got %+v want %+v", got, tt.in)
			}
			switch {
			case tt.in.Quantity == nil && got.Quantity != nil:
				t.Fatalf("Quantity 가 nil 이어야 함: %v", *got.Quantity)
			case tt.in.Quantity != nil && (got.Quantity == nil || *got.Quantity != *tt.in.Quantity):
				t.Fatalf("Quantity 불일치: got %v want %v", got.Quantity, *tt.in.Quantity)
			}
		})
	}
}
