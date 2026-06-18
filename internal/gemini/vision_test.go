package gemini

import (
	"reflect"
	"testing"
)

func TestDedupeNames(t *testing.T) {
	t.Parallel()
	got := dedupeNames([]string{"휴지", "휴지", "물티슈", "", "  휴지  ", "정리함"})
	want := []string{"휴지", "물티슈", "정리함"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeNames = %v, want %v", got, want)
	}
}

func TestDedupeNames_empty(t *testing.T) {
	t.Parallel()
	if got := dedupeNames([]string{"", "   "}); len(got) != 0 {
		t.Fatalf("빈 입력 = %v, want []", got)
	}
}
