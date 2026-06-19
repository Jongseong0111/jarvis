package claudecode_test

import (
	"testing"

	"github.com/Jongseong0111/jarvis/internal/claudecode"
)

func TestParseOutput_success(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"result","subtype":"success","session_id":"ses_abc","result":"개념 정리 결과입니다.","is_error":false}`)
	got, err := claudecode.ParseOutput(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "ses_abc" {
		t.Errorf("session_id: got %q, want %q", got.SessionID, "ses_abc")
	}
	if got.Text != "개념 정리 결과입니다." {
		t.Errorf("text: got %q, want %q", got.Text, "개념 정리 결과입니다.")
	}
}

func TestParseOutput_invalid(t *testing.T) {
	t.Parallel()
	_, err := claudecode.ParseOutput([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseOutput_isError(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"result","subtype":"error","session_id":"ses_err","result":"permission denied","is_error":true}`)
	_, err := claudecode.ParseOutput(data)
	if err == nil {
		t.Fatal("is_error=true 인데 에러 반환 안 됨")
	}
}
