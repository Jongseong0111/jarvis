package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSource_sameURLUpdatesInPlace(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	url := "https://chatgpt.com/share/abc"
	p1, err := WriteSource(repo, "2026-06-18", "고랭 장점 설명", url, "## 초안")
	if err != nil {
		t.Fatalf("WriteSource 1: %v", err)
	}
	// 같은 URL 로 다시 저장(수정본) → 새 파일이 아니라 같은 파일 갱신이어야 함
	p2, err := WriteSource(repo, "2026-06-18", "고랭 장점 설명", url, "## 수정본\n- 채널")
	if err != nil {
		t.Fatalf("WriteSource 2: %v", err)
	}
	if p2 != p1 {
		t.Fatalf("같은 URL 재저장이 새 파일 생성: %q != %q", p2, p1)
	}
	b, _ := os.ReadFile(p1)
	if !strings.Contains(string(b), "## 수정본") || strings.Contains(string(b), "## 초안") {
		t.Fatalf("내용 갱신 안 됨:\n%s", b)
	}
	// conversation 디렉터리에 파일이 하나뿐이어야 함
	entries, _ := os.ReadDir(filepath.Join(repo, "sources", "conversation"))
	if len(entries) != 1 {
		t.Fatalf("파일 수 = %d, want 1", len(entries))
	}
}

func TestWriteSource_differentURLSameTitleGetsNewFile(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	p1, _ := WriteSource(repo, "2026-06-18", "공통제목", "https://chatgpt.com/share/aaa", "a")
	p2, err := WriteSource(repo, "2026-06-18", "공통제목", "https://chatgpt.com/share/bbb", "b")
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	if p2 == p1 {
		t.Fatal("다른 URL 인데 같은 파일로 덮어씀")
	}
	if !strings.HasSuffix(p2, "공통제목-2.md") {
		t.Fatalf("다른 URL·같은 제목은 -2 여야 함: %q", p2)
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"고랭 장점 설명":       "고랭-장점-설명",
		"Hello, World!":  "hello-world",
		"  공백  많은   제목 ": "공백-많은-제목",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSource_writesFrontmatterAndBody(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	path, err := WriteSource(repo, "2026-06-18", "고랭 장점 설명", "https://chatgpt.com/share/abc", "## 핵심\n- 고루틴")
	if err != nil {
		t.Fatalf("WriteSource: %v", err)
	}
	want := filepath.Join(repo, "sources", "conversation", "2026-06-18-고랭-장점-설명.md")
	if path != want {
		t.Fatalf("경로 = %q, want %q", path, want)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	for _, sub := range []string{"title: 고랭 장점 설명", "source: chatgpt-share", "url: https://chatgpt.com/share/abc", "captured: 2026-06-18", "type: conversation", "## 핵심", "- 고루틴"} {
		if !strings.Contains(s, sub) {
			t.Fatalf("본문에 %q 없음:\n%s", sub, s)
		}
	}
}

func TestWriteSource_duplicateGetsSuffix(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	_, _ = WriteSource(repo, "2026-06-18", "중복", "", "a")
	path2, err := WriteSource(repo, "2026-06-18", "중복", "", "b")
	if err != nil {
		t.Fatalf("WriteSource 2: %v", err)
	}
	if !strings.HasSuffix(path2, "2026-06-18-중복-2.md") {
		t.Fatalf("중복 접미 실패: %q", path2)
	}
}

func TestWriteSource_omitsEmptyURL(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	path, _ := WriteSource(repo, "2026-06-18", "노URL", "", "본문")
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "url:") {
		t.Fatalf("빈 URL 은 frontmatter 에서 빠져야 함:\n%s", b)
	}
}
