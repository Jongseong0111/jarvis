package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestService_SaveSource_writesFile(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	svc := NewService(nil, repo) // 저장 경로는 summarizer 안 씀
	path, err := svc.SaveSource(context.Background(), "테스트 제목", "", "본문 내용")
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("파일 미생성: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(repo, "sources", "conversation") {
		t.Fatalf("저장 위치 = %q", filepath.Dir(path))
	}
}
