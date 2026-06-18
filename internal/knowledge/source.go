package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// WriteSource 는 요약을 sources/conversation/<today>-<slug>.md 로 저장한다(미커밋).
// url 이 비면 frontmatter 에서 생략한다.
// 같은 url 의 소스가 이미 있으면 그 파일을 갱신(덮어쓰기)한다 — 같은 대화를 다시 저장(수정)하는 경우.
// 그 외 같은 경로가 있으면 -2,-3 접미를 붙여 새 파일을 만든다(다른 대화·같은 제목).
func WriteSource(repoPath, today, title, url, content string) (string, error) {
	dir := filepath.Join(repoPath, "sources", "conversation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("디렉터리 생성 실패: %w", err)
	}

	// 같은 url 의 기존 소스가 있으면 그 경로를 재사용(갱신).
	path := ""
	if url != "" {
		if existing, ok := findSourceByURL(dir, url); ok {
			path = existing
		}
	}
	if path == "" {
		slug := slugify(title)
		if slug == "" {
			slug = "untitled"
		}
		base := today + "-" + slug
		path = filepath.Join(dir, base+".md")
		for i := 2; fileExists(path); i++ {
			path = filepath.Join(dir, fmt.Sprintf("%s-%d.md", base, i))
		}
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + title + "\n")
	b.WriteString("source: chatgpt-share\n")
	if url != "" {
		b.WriteString("url: " + url + "\n")
	}
	b.WriteString("captured: " + today + "\n")
	b.WriteString("type: conversation\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("파일 쓰기 실패: %w", err)
	}
	return path, nil
}

// slugify 는 제목을 파일명용 슬러그로 바꾼다(한글 유지, 영문 소문자, 비단어→'-', 60룬 컷).
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if r := []rune(out); len(r) > 60 {
		out = strings.Trim(string(r[:60]), "-")
	}
	return out
}

// findSourceByURL 은 dir 안에서 frontmatter 의 `url:` 이 url 과 일치하는 첫 .md 파일을 찾는다.
func findSourceByURL(dir, url string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	want := "url: " + url
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimRight(line, "\r") == want {
				return p, true
			}
		}
	}
	return "", false
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
