package router

import (
	"strings"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

func Test_validateIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want domain.Intent
	}{
		{name: "유효 intent", in: "home.add", want: domain.IntentHomeAdd},
		{name: "앞뒤 공백 trim", in: "  knowledge.update\n", want: domain.IntentKnowledgeUpdate},
		{name: "미지의 값 → unknown", in: "weather.today", want: domain.IntentUnknown},
		{name: "빈 문자열 → unknown", in: "", want: domain.IntentUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := validateIntent(tt.in); got != tt.want {
				t.Fatalf("validateIntent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func Test_enumValues(t *testing.T) {
	t.Parallel()
	got := enumValues()
	if len(got) != len(domain.AllIntents()) {
		t.Fatalf("enumValues 길이 = %d, want %d", len(got), len(domain.AllIntents()))
	}
	// 모든 값이 유효 intent 로 다시 검증되는지 확인
	for _, v := range got {
		if validateIntent(v) == domain.IntentUnknown && v != string(domain.IntentUnknown) {
			t.Fatalf("enumValues 에 비유효 값 포함: %q", v)
		}
	}
}

func Test_buildClassifyPrompt(t *testing.T) {
	t.Parallel()
	p := buildClassifyPrompt("건전지 어디 뒀지?")
	if !strings.Contains(p, "건전지 어디 뒀지?") {
		t.Fatal("프롬프트에 사용자 메시지가 포함되어야 함")
	}
	if !strings.Contains(p, string(domain.IntentUnknown)) {
		t.Fatal("프롬프트에 system.unknown 안내가 포함되어야 함")
	}
	if !strings.Contains(p, string(domain.IntentHomeAdd)) {
		t.Fatal("프롬프트에 intent 목록이 포함되어야 함")
	}
}
