package computeruse

import "fmt"

// iouDedupThreshold 视觉元素与 a11y 元素的 IoU 判重阈值（UFO² H2：>0.1 视为同一元素）。
const iouDedupThreshold = 0.1

// IoU 计算两个包围盒的交并比；任一零面积返回 0。
func IoU(a, b Rect) float64 {
	x1 := max(a.X, b.X)
	y1 := max(a.Y, b.Y)
	x2 := min(a.X+a.W, b.X+b.W)
	y2 := min(a.Y+a.H, b.Y+b.H)
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	inter := float64((x2 - x1) * (y2 - y1))
	areaA := float64(a.W * a.H)
	areaB := float64(b.W * b.H)
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

// MergeA11yVision 融合 a11y 主表与 vision 补充表（UFO² H2 算法）：
// a11y 元素原样保留且优先；vision 元素与任一 a11y 元素 IoU > 0.1 时视为重复丢弃。
// 保留的 vision 元素统一 Source=vision、Generation 对齐 gen、Ref 重编为 g{gen}.v{i}
// （v 前缀与 a11y 的 e 前缀区分，避免同代 ref 冲突）。
func MergeA11yVision(a11y, vision []UIElement, gen int) []UIElement {
	merged := make([]UIElement, 0, len(a11y)+len(vision))
	merged = append(merged, a11y...)
	v := 0
	for _, el := range vision {
		dup := false
		for _, a := range a11y {
			if IoU(a.BBox, el.BBox) > iouDedupThreshold {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		el.Source = "vision"
		el.Generation = gen
		el.Ref = fmt.Sprintf("g%d.v%d", gen, v)
		v++
		merged = append(merged, el)
	}
	return merged
}
