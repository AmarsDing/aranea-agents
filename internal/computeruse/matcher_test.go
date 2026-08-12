package computeruse

import (
	"testing"

	bizcomputeruse "aranea-agents/internal/biz/computeruse"
)

// el 构造可交互元素的快捷方式。
func el(ref, name string) bizcomputeruse.UIElement {
	return bizcomputeruse.UIElement{Ref: ref, Name: name, Interactivity: true, Enabled: true}
}

// TestMatcherMatch 表驱动：归一化 → 精确 → 包含 → 编辑距离，含防歧义。
func TestMatcherMatch(t *testing.T) {
	cases := []struct {
		name     string
		elements []bizcomputeruse.UIElement
		target   string
		wantRef  string // 空串 = 期望未命中（nil）
	}{
		{
			name:     "精确命中",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "保存"), el("g1.e2", "打开")},
			target:   "保存",
			wantRef:  "g1.e1",
		},
		{
			name:     "归一化：大小写与空白与标点",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "Save As...")},
			target:   "save as",
			wantRef:  "g1.e1",
		},
		{
			name:     "归一化：全角转半角",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "保存（Ｓ）")},
			target:   "保存(S)",
			wantRef:  "g1.e1",
		},
		{
			name:     "包含匹配",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "另存为对话框"), el("g1.e2", "取消")},
			target:   "另存为",
			wantRef:  "g1.e1",
		},
		{
			name:     "精确优先于包含（分差≥0.2 判命中）",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "保存"), el("g1.e2", "保存文档")},
			target:   "保存",
			wantRef:  "g1.e1",
		},
		{
			name:     "编辑距离≤2 容错",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "按钮")},
			target:   "按扭",
			wantRef:  "g1.e1",
		},
		{
			name:     "歧义返回 nil（top1 与 top2 分差 <0.2）",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "保存文档"), el("g1.e2", "保存文件")},
			target:   "保存",
			wantRef:  "",
		},
		{
			name: "不可交互元素跳过",
			elements: []bizcomputeruse.UIElement{
				{Ref: "g1.e1", Name: "保存", Interactivity: false, Enabled: true},
			},
			target:  "保存",
			wantRef: "",
		},
		{
			name: "禁用元素跳过",
			elements: []bizcomputeruse.UIElement{
				{Ref: "g1.e1", Name: "保存", Interactivity: true, Enabled: false},
			},
			target:  "保存",
			wantRef: "",
		},
		{
			name:     "目标为空",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "保存")},
			target:   "  ",
			wantRef:  "",
		},
		{
			name:     "元素为空",
			elements: nil,
			target:   "保存",
			wantRef:  "",
		},
		{
			name:     "距离过远不命中",
			elements: []bizcomputeruse.UIElement{el("g1.e1", "完全不相干的东西")},
			target:   "保存",
			wantRef:  "",
		},
	}

	m := NewMatcher()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit := m.Match(tc.elements, tc.target)
			if tc.wantRef == "" {
				if hit != nil {
					t.Errorf("Match = %+v, want nil", hit)
				}
				return
			}
			if hit == nil {
				t.Fatalf("Match = nil, want ref %q", tc.wantRef)
			}
			if hit.Ref != tc.wantRef {
				t.Errorf("Match.Ref = %q, want %q", hit.Ref, tc.wantRef)
			}
		})
	}
}

// TestMatcherReturnsCopy 返回副本，调用方修改不影响原切片。
func TestMatcherReturnsCopy(t *testing.T) {
	elements := []bizcomputeruse.UIElement{el("g1.e1", "保存")}
	hit := NewMatcher().Match(elements, "保存")
	if hit == nil {
		t.Fatal("expect hit")
	}
	hit.Name = "改动"
	if elements[0].Name != "保存" {
		t.Error("Match 应返回副本而非切片内指针")
	}
}

// TestNormalize 归一化单测。
func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"保存（Ｓ）":   "保存s",
		" Save As ": "saveas",
		"Ctrl＋Ｃ":   "ctrlc",
		"":         "",
		"　":        "", // 全角空格
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLevenshtein rune 级编辑距离单测。
func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"按钮", "按扭", 1},
		{"保存", "另存", 1},
		{"kitten", "sitting", 3},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
