package computeruse

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// DownscalePNG 将 PNG 按最长边等比缩小到 maxSide 内（已小于则原样返回）。
// 返回缩放后 PNG 字节与缩放因子（新/旧；未缩放为 1）。VLM 调用前降采样：
// 减少视觉 token 数与 prefill 耗时，grounding 语义坐标由归一化/同比例 bbox 保持。
func DownscalePNG(pngBytes []byte, maxSide int) ([]byte, float64, error) {
	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("computeruse: 降采样解码失败: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := max(w, h)
	if longest <= maxSide || maxSide <= 0 {
		return pngBytes, 1.0, nil
	}
	factor := float64(maxSide) / float64(longest)
	nw, nh := int(float64(w)*factor), int(float64(h)*factor)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, 0, fmt.Errorf("computeruse: 降采样编码失败: %w", err)
	}
	return buf.Bytes(), factor, nil
}

// SoM（Set-of-Mark）标注样式：红框 + 红底白字编号（白字在红底上对比度最高，
// 编号置于框左上角，与 VLM prompt 中候选列表序号一一对应）。
var (
	somBoxColor  = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	somTextColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

// AnnotateSoM 在截图上为候选元素绘制编号框（Set-of-Mark）。
// 编号从 1 开始按 candidates 顺序；越界 bbox 自动 clamp，完全在图外的跳过绘制。
// 返回标注后的 PNG 字节。
func AnnotateSoM(img Image, candidates []UIElement) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(img.PNG))
	if err != nil {
		return nil, fmt.Errorf("computeruse: SoM 解码截图失败: %w", err)
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	for i, el := range candidates {
		r, ok := clampRect(el.BBox, bounds)
		if !ok {
			continue
		}
		drawBox(dst, r)
		drawLabel(dst, r, i+1)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("computeruse: SoM 编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// clampRect 将物理像素 bbox 裁剪到图内；完全在外返回 false。
func clampRect(b Rect, bounds image.Rectangle) (image.Rectangle, bool) {
	x1 := max(b.X, bounds.Min.X)
	y1 := max(b.Y, bounds.Min.Y)
	x2 := min(b.X+b.W, bounds.Max.X)
	y2 := min(b.Y+b.H, bounds.Max.Y)
	if x2 <= x1 || y2 <= y1 {
		return image.Rectangle{}, false
	}
	return image.Rect(x1, y1, x2, y2), true
}

// drawBox 绘制 2px 宽矩形边框。
func drawBox(dst *image.RGBA, r image.Rectangle) {
	for d := 0; d < 2; d++ {
		for x := r.Min.X + d; x < r.Max.X-d; x++ {
			dst.Set(x, r.Min.Y+d, somBoxColor)
			dst.Set(x, r.Max.Y-1-d, somBoxColor)
		}
		for y := r.Min.Y + d; y < r.Max.Y-d; y++ {
			dst.Set(r.Min.X+d, y, somBoxColor)
			dst.Set(r.Max.X-1-d, y, somBoxColor)
		}
	}
}

// drawLabel 在框左上角绘制红底白字编号。
func drawLabel(dst *image.RGBA, r image.Rectangle, n int) {
	label := fmt.Sprintf("%d", n)
	face := basicfont.Face7x13
	w := font.MeasureString(face, label).Ceil() + 4
	h := 15
	// 标签背景：优先放框内顶部；框太窄时仍绘制（可能稍溢出框右缘，可接受）。
	bg := image.Rect(r.Min.X, r.Min.Y, min(r.Min.X+w, dst.Bounds().Max.X), min(r.Min.Y+h, dst.Bounds().Max.Y))
	draw.Draw(dst, bg, &image.Uniform{C: somBoxColor}, image.Point{}, draw.Src)
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(somTextColor),
		Face: face,
		Dot:  fixed.P(r.Min.X+2, r.Min.Y+12),
	}
	d.DrawString(label)
}
