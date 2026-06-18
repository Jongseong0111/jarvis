package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"고랭 장점 설명":     "고랭-장점-설명",
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
