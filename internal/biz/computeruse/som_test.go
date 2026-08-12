package computeruse

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func newTestImage(t *testing.T, w, h int) Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// 填充白色底，便于验证框线像素变化。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return Image{PNG: buf.Bytes(), Width: w, Height: h, ScaleFactor: 1.0}
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode annotated png: %v", err)
	}
	return img
}

func TestAnnotateSoM_DrawsBoxes(t *testing.T) {
	img := newTestImage(t, 100, 100)
	cands := []UIElement{
		{Ref: "g1.e0", Name: "按钮", BBox: Rect{X: 10, Y: 10, W: 30, H: 20}},
		{Ref: "g1.e1", Name: "输入框", BBox: Rect{X: 50, Y: 60, W: 40, H: 20}},
	}
	out, err := AnnotateSoM(img, cands)
	if err != nil {
		t.Fatalf("AnnotateSoM: %v", err)
	}
	got := decode(t, out)
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 100 {
		t.Fatalf("尺寸变化: %v", got.Bounds())
	}
	// 第一个框的顶边中点应非白色（框线已绘制）。
	r, g, b, _ := got.At(25, 10).RGBA()
	if r == 0xffff && g == 0xffff && b == 0xffff {
		t.Fatalf("框线未绘制：At(25,10) 仍为白色")
	}
	// 框外远离区域保持白色。
	r, g, b, _ = got.At(5, 5).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("框外像素被污染: At(5,5)")
	}
}

func TestAnnotateSoM_EmptyCandidates(t *testing.T) {
	img := newTestImage(t, 40, 40)
	out, err := AnnotateSoM(img, nil)
	if err != nil {
		t.Fatalf("AnnotateSoM nil: %v", err)
	}
	got := decode(t, out)
	// 无候选时不绘制任何框：中心像素仍白。
	r, g, b, _ := got.At(20, 20).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("空候选不应绘制")
	}
}

func TestAnnotateSoM_ClampOutOfBounds(t *testing.T) {
	img := newTestImage(t, 50, 50)
	cands := []UIElement{
		{Ref: "g1.e0", BBox: Rect{X: -20, Y: -20, W: 30, H: 30}}, // 部分越界
		{Ref: "g1.e1", BBox: Rect{X: 40, Y: 40, W: 100, H: 100}}, // 大部分越界
		{Ref: "g1.e2", BBox: Rect{X: 200, Y: 200, W: 10, H: 10}}, // 完全在外
	}
	if _, err := AnnotateSoM(img, cands); err != nil {
		t.Fatalf("越界元素不应报错: %v", err)
	}
}

func TestAnnotateSoM_InvalidPNG(t *testing.T) {
	img := Image{PNG: []byte("not a png"), Width: 10, Height: 10}
	if _, err := AnnotateSoM(img, nil); err == nil {
		t.Fatalf("非法 PNG 应返回错误")
	}
}
