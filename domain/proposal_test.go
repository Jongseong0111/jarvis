package domain

import "testing"

func TestChangeProposal_JSONRoundtrip(t *testing.T) {
	t.Parallel()
	in := ChangeProposal{
		Op:      "add_item",
		Summary: "체온계 → 아기 트롤리",
		Fields: map[string]string{
			"name":          "체온계",
			"location_id":   "loc-1",
			"location_name": "아기 트롤리",
			"zone":          "거실",
		},
	}
	got, err := DecodeProposal(in.Encode())
	if err != nil {
		t.Fatalf("DecodeProposal: %v", err)
	}
	if got.Op != in.Op || got.Summary != in.Summary {
		t.Fatalf("op/summary 불일치: %+v", got)
	}
	for k, v := range in.Fields {
		if got.Fields[k] != v {
			t.Fatalf("Fields[%q] = %q, want %q", k, got.Fields[k], v)
		}
	}
}
