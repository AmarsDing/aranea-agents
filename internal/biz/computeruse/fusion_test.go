package computeruse

import "testing"

func TestIoU(t *testing.T) {
	cases := []struct {
		name string
		a, b Rect
		want float64
	}{
		{"identical", Rect{X: 0, Y: 0, W: 10, H: 10}, Rect{X: 0, Y: 0, W: 10, H: 10}, 1.0},
		{"no_overlap", Rect{X: 0, Y: 0, W: 10, H: 10}, Rect{X: 20, Y: 20, W: 5, H: 5}, 0.0},
		{"half_overlap", Rect{X: 0, Y: 0, W: 10, H: 10}, Rect{X: 5, Y: 0, W: 10, H: 10}, 1.0 / 3.0},
		{"zero_area_a", Rect{X: 0, Y: 0, W: 0, H: 0}, Rect{X: 0, Y: 0, W: 10, H: 10}, 0.0},
		{"contained", Rect{X: 0, Y: 0, W: 10, H: 10}, Rect{X: 2, Y: 2, W: 4, H: 4}, 0.16},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IoU(c.a, c.b)
			if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("IoU(%v,%v)=%v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestMergeA11yVision_A11yPreferred(t *testing.T) {
	a11y := []UIElement{
		{Ref: "g1.e0", Name: "保存", BBox: Rect{X: 10, Y: 10, W: 40, H: 20}, Source: "uia", Generation: 1},
	}
	vision := []UIElement{
		// 与 a11y 元素 IoU > 0.1 → 视为重复，丢弃。
		{Name: "保存图标", BBox: Rect{X: 12, Y: 12, W: 40, H: 20}, Source: "vision"},
	}
	merged := MergeA11yVision(a11y, vision, 1)
	if len(merged) != 1 {
		t.Fatalf("merged len=%d, want 1（重复 vision 元素应丢弃）", len(merged))
	}
	if merged[0].Ref != "g1.e0" {
		t.Fatalf("merged[0].Ref=%q, want g1.e0（a11y 优先）", merged[0].Ref)
	}
}

func TestMergeA11yVision_VisionSupplement(t *testing.T) {
	a11y := []UIElement{
		{Ref: "g3.e0", Name: "文件", BBox: Rect{X: 0, Y: 0, W: 30, H: 15}, Source: "uia", Generation: 3},
	}
	vision := []UIElement{
		{Name: "自绘按钮", BBox: Rect{X: 100, Y: 100, W: 50, H: 30}},
		{Name: "另一个图标", BBox: Rect{X: 200, Y: 100, W: 20, H: 20}, Source: "omniparser"},
	}
	merged := MergeA11yVision(a11y, vision, 3)
	if len(merged) != 3 {
		t.Fatalf("merged len=%d, want 3", len(merged))
	}
	// vision 元素：Source 统一 vision、Generation 对齐、Ref 重编 v 前缀避免与 a11y 冲突。
	wantRefs := []string{"g3.v0", "g3.v1"}
	for i, el := range merged[1:] {
		if el.Source != "vision" {
			t.Fatalf("vision el[%d].Source=%q, want vision", i, el.Source)
		}
		if el.Generation != 3 {
			t.Fatalf("vision el[%d].Generation=%d, want 3", i, el.Generation)
		}
		if el.Ref != wantRefs[i] {
			t.Fatalf("vision el[%d].Ref=%q, want %q", i, el.Ref, wantRefs[i])
		}
	}
}

func TestMergeA11yVision_ThresholdBoundary(t *testing.T) {
	// IoU 恰好 == 0.1 不丢弃（>0.1 才判重）。
	a11y := []UIElement{
		{Ref: "g1.e0", BBox: Rect{X: 0, Y: 0, W: 10, H: 10}, Source: "uia"},
	}
	// 偏移 9：交 1*10=10，并 190 → ≈0.0526<0.1 保留。
	vision := []UIElement{
		{Name: "keep", BBox: Rect{X: 9, Y: 0, W: 10, H: 10}},
	}
	merged := MergeA11yVision(a11y, vision, 1)
	if len(merged) != 2 {
		t.Fatalf("IoU<0.1 的 vision 元素应保留，merged len=%d", len(merged))
	}
}

func TestMergeA11yVision_NilEmpty(t *testing.T) {
	if got := MergeA11yVision(nil, nil, 1); len(got) != 0 {
		t.Fatalf("nil+nil 应返回空，got %d", len(got))
	}
	a11y := []UIElement{{Ref: "g1.e0", BBox: Rect{W: 5, H: 5}}}
	if got := MergeA11yVision(a11y, nil, 1); len(got) != 1 {
		t.Fatalf("nil vision 应原样返回 a11y，got %d", len(got))
	}
}
