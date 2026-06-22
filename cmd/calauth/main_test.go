package main

import "testing"

func TestRandomState(t *testing.T) {
	t.Parallel()
	a, err := randomState()
	if err != nil {
		t.Fatalf("randomState: %v", err)
	}
	b, _ := randomState()
	if a == "" || a == b {
		t.Fatalf("state 가 비었거나 매번 같음: %q vs %q", a, b)
	}
}
