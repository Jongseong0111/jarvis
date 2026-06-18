package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // png 디코더 등록(image.Decode 용)
	"strings"

	xdraw "golang.org/x/image/draw"
	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
)

// visionPrompt 는 사진에서 정리/수납 대상 물건 이름만 뽑게 지시한다.
const visionPrompt = `이 사진들에 보이는, 옮기거나 수납할 수 있는 물건들을 한국어 이름의 JSON 배열로만 반환해라.
가구·벽·바닥·문·창문 같은 배경 구조물은 제외하고, 정리하거나 수납할 수 있는 물건만 포함해라.
같은 물건이 여러 개여도 이름은 한 번만. 확실하지 않으면 제외한다.`

// visionMaxDim 은 비전에 보내기 전 이미지 긴 변의 최대 픽셀이다.
// 폰 사진(수 MB)을 그대로 보내면 비전 호출이 느려/타임아웃나므로 서버에서 축소한다.
const visionMaxDim = 1024

// ExtractItems 는 이미지들에서 물건 이름 목록을 추출한다(비전 모델 사용).
func (c *Client) ExtractItems(ctx context.Context, images []domain.Image) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini 클라이언트 생성 실패: %w", err)
	}

	parts := []*genai.Part{{Text: visionPrompt}}
	for _, img := range images {
		img = downscaleForVision(img, visionMaxDim) // 큰 사진은 보내기 전 축소(속도/비용/타임아웃)
		parts = append(parts, &genai.Part{InlineData: &genai.Blob{Data: img.Data, MIMEType: img.MIME}})
	}
	contents := []*genai.Content{{Role: genai.RoleUser, Parts: parts}}

	temp := float32(0)
	thinkBudget := int32(0) // thinking 비활성(속도/비용)
	cfg := &genai.GenerateContentConfig{
		Temperature:      &temp,
		ThinkingConfig:   &genai.ThinkingConfig{ThinkingBudget: &thinkBudget},
		ResponseMIMEType: "application/json",
		ResponseSchema:   &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	}

	resp, err := client.Models.GenerateContent(ctx, c.model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini 비전 생성 실패: %w", err)
	}

	var names []string
	if err := json.Unmarshal([]byte(resp.Text()), &names); err != nil {
		return nil, fmt.Errorf("비전 응답 파싱 실패: %w (raw=%q)", err, resp.Text())
	}
	return dedupeNames(names), nil
}

// downscaleForVision 은 긴 변이 maxDim 을 넘으면 비율을 유지해 축소하고 JPEG 로 재인코딩한다.
// 디코드 불가(예: HEIC) 또는 이미 충분히 작은 이미지는 원본을 그대로 돌려준다.
func downscaleForVision(img domain.Image, maxDim int) domain.Image {
	src, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return img // 디코드 불가 포맷 → 원본 그대로(비전이 받아줄 수도 있음)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img // 이미 충분히 작음
	}

	nw, nh := w, h
	if w >= h {
		nw, nh = maxDim, h*maxDim/w
	} else {
		nw, nh = w*maxDim/h, maxDim
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return img // 재인코딩 실패 시 원본 유지
	}
	return domain.Image{Data: buf.Bytes(), MIME: "image/jpeg"}
}

// dedupeNames 는 공백 제거 후 빈 값/중복을 걸러 순서를 보존한 목록을 만든다.
func dedupeNames(names []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
