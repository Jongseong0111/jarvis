package log

import (
	"context"
	"testing"
)

func TestFromContext_기본로거_폴백(t *testing.T) {
	t.Parallel()
	if got := FromContext(context.Background()); got == nil {
		t.Fatal("FromContext 가 nil 을 반환하면 안 됨")
	}
}

func TestWithContext_라운드트립(t *testing.T) {
	t.Parallel()
	logger := New("local")
	ctx := WithContext(context.Background(), logger)
	if got := FromContext(ctx); got != logger {
		t.Fatal("FromContext 가 주입한 로거를 반환해야 함")
	}
}
