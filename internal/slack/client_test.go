package slack

import "testing"

func TestIsImageMime(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/webp":      true,
		"application/pdf": false,
		"text/plain":      false,
		"":                false,
	}
	for mime, want := range cases {
		if got := isImageMime(mime); got != want {
			t.Errorf("isImageMime(%q) = %v, want %v", mime, got, want)
		}
	}
}
