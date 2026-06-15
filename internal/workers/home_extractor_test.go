package workers

import (
	"strings"
	"testing"
)

func Test_buildExtractPrompt(t *testing.T) {
	t.Parallel()
	p := buildExtractPrompt("아기 트롤리에 체온계 뒀어",
		[]string{"아기 트롤리", "베란다 수납장1"},
		[]string{"아기상비약", "세탁용품"})
	for _, want := range []string{"아기 트롤리에 체온계 뒀어", "베란다 수납장1", "아기상비약", "action", "quantity"} {
		if !strings.Contains(p, want) {
			t.Fatalf("프롬프트에 %q 가 없음", want)
		}
	}
}

func Test_extractSchema(t *testing.T) {
	t.Parallel()
	s := extractSchema()
	if s.Properties["action"] == nil || len(s.Properties["action"].Enum) != 2 {
		t.Fatal("action enum(add/search) 정의 필요")
	}
	if s.Properties["quantity"] == nil || s.Properties["quantity"].Nullable == nil || !*s.Properties["quantity"].Nullable {
		t.Fatal("quantity 는 nullable 정수여야 함")
	}
}
