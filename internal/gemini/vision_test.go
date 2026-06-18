package gemini

import (
	"bytes"
	"image"
	"image/jpeg"
	"reflect"
	"testing"

	"github.com/Jongseong0111/jarvis/domain"
)

// makeJPEG 는 w×h 크기의 더미 JPEG 바이트를 만든다(테스트용).
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestDownscaleForVision_shrinksLargeImage(t *testing.T) {
	t.Parallel()
	in := domain.Image{Data: makeJPEG(t, 2000, 1500), MIME: "image/jpeg"}
	out := downscaleForVision(in, 1024)

	img, _, err := image.Decode(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatalf("결과 디코드 실패: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 1024 || b.Dy() != 768 {
		t.Fatalf("축소 크기 = %dx%d, want 1024x768 (비율 유지)", b.Dx(), b.Dy())
	}
	if len(out.Data) >= len(in.Data) {
		t.Fatalf("축소 후 바이트가 더 큼: %d >= %d", len(out.Data), len(in.Data))
	}
	if out.MIME != "image/jpeg" {
		t.Fatalf("MIME = %q, want image/jpeg", out.MIME)
	}
}

func TestDownscaleForVision_keepsSmallImage(t *testing.T) {
	t.Parallel()
	in := domain.Image{Data: makeJPEG(t, 800, 600), MIME: "image/jpeg"}
	out := downscaleForVision(in, 1024)
	if !bytes.Equal(out.Data, in.Data) {
		t.Fatal("이미 작은 이미지는 원본 그대로여야 함")
	}
}

func TestDownscaleForVision_undecodableReturnedAsIs(t *testing.T) {
	t.Parallel()
	in := domain.Image{Data: []byte("not an image"), MIME: "image/heic"}
	out := downscaleForVision(in, 1024)
	if !bytes.Equal(out.Data, in.Data) || out.MIME != in.MIME {
		t.Fatal("디코드 불가 이미지는 원본 그대로여야 함")
	}
}

func TestDedupeNames(t *testing.T) {
	t.Parallel()
	got := dedupeNames([]string{"휴지", "휴지", "물티슈", "", "  휴지  ", "정리함"})
	want := []string{"휴지", "물티슈", "정리함"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeNames = %v, want %v", got, want)
	}
}

func TestDedupeNames_empty(t *testing.T) {
	t.Parallel()
	if got := dedupeNames([]string{"", "   "}); len(got) != 0 {
		t.Fatalf("빈 입력 = %v, want []", got)
	}
}
