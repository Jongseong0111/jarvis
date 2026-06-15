package domain

import "testing"

func TestIntent_Namespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		intent Intent
		want   string
	}{
		{IntentHomeAdd, "home"},
		{IntentKnowledgeUpdate, "knowledge"},
		{IntentSystemHelp, "system"},
		{IntentUnknown, "system"},
		{Intent("nodot"), "nodot"},
	}
	for _, tt := range tests {
		t.Run(string(tt.intent), func(t *testing.T) {
			t.Parallel()
			if got := tt.intent.Namespace(); got != tt.want {
				t.Fatalf("Namespace(%q) = %q, want %q", tt.intent, got, tt.want)
			}
		})
	}
}

func TestAllIntents_unique(t *testing.T) {
	t.Parallel()
	seen := map[Intent]bool{}
	for _, i := range AllIntents() {
		if seen[i] {
			t.Fatalf("중복 intent: %q", i)
		}
		seen[i] = true
	}
}
