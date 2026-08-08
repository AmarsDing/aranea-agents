package blockparse

import (
	"strings"
	"testing"
)

// TestAppendHeadingAnchor 基本：ATX heading 命中路径 → 行尾追加锚点；
// 二次调用幂等（已锚跳过）。
func TestAppendHeadingAnchor(t *testing.T) {
	md := "# Alpha\n\n正文。\n\n## Beta\n\n子节。\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Alpha", "Beta"}, "x1y2z3")
	if !changed {
		t.Fatal("changed = false, want true")
	}
	want := "# Alpha\n\n正文。\n\n## Beta ^x1y2z3\n\n子节。\n"
	if string(got) != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	// 幂等：再次回填同一路径不再变更。
	again, changed2 := AppendHeadingAnchor(got, []string{"Alpha", "Beta"}, "another")
	if changed2 {
		t.Errorf("已锚块二次回填应跳过，got:\n%q", again)
	}
	if string(again) != want {
		t.Errorf("幂等调用改变文本: %q", again)
	}
	// 回填后重新解析：锚点进入 AST（与 Parse 口径互认）。
	blocks, _ := parse(t, want)
	if blocks[2].Kind != KindHeading || blocks[2].Anchor != "x1y2z3" {
		t.Errorf("回填后解析 blocks[2] kind/anchor = %v/%q, want heading/x1y2z3", blocks[2].Kind, blocks[2].Anchor)
	}
}

// TestAppendHeadingAnchor_FirstDuplicate 重复标题取首（与 Resolver/存储层
// ordinal 最小口径一致）：锚首个未锚命中块；首个已锚时不顺延到第二个。
func TestAppendHeadingAnchor_FirstDuplicate(t *testing.T) {
	md := "# Dup\n\n甲。\n\n# Dup\n\n乙。\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Dup"}, "aa11")
	if !changed {
		t.Fatal("changed = false")
	}
	if !strings.Contains(string(got), "# Dup ^aa11\n\n甲。") {
		t.Errorf("应锚第一个 Dup:\n%q", got)
	}
	if strings.Count(string(got), "^aa11") != 1 {
		t.Errorf("锚点应只出现一次:\n%q", got)
	}
	// 首个已锚：第二个同名块不被顺延回填（与 FindBlockByHeadingPath 取首一致）。
	again, changed2 := AppendHeadingAnchor(got, []string{"Dup"}, "bb22")
	if changed2 {
		t.Errorf("首个已锚应整体跳过（不顺延）:\n%q", again)
	}
}

// TestAppendHeadingAnchor_AlreadyAnchored 已有显式锚 → 跳过。
func TestAppendHeadingAnchor_AlreadyAnchored(t *testing.T) {
	md := "## Foo ^old\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Foo"}, "new1")
	if changed {
		t.Errorf("已锚块不应变更: %q", got)
	}
	if string(got) != md {
		t.Errorf("文本被改写: %q", got)
	}
}

// TestAppendHeadingAnchor_ClosingHashes ATX 闭合符：锚插在闭合 ## 之前，
// 保证重新解析仍识别（插在行尾会被闭合符吞掉）。
func TestAppendHeadingAnchor_ClosingHashes(t *testing.T) {
	md := "## Foo ##\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Foo"}, "c1")
	if !changed {
		t.Fatal("changed = false")
	}
	if string(got) != "## Foo ^c1 ##\n" {
		t.Errorf("got %q, want 锚在闭合符前", got)
	}
	blocks, _ := parse(t, string(got))
	if blocks[0].Anchor != "c1" {
		t.Errorf("回填后解析 Anchor = %q", blocks[0].Anchor)
	}
}

// TestAppendHeadingAnchor_Setext Setext heading：锚插在文本行末（=== 之前）。
func TestAppendHeadingAnchor_Setext(t *testing.T) {
	md := "Foo Bar\n===\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Foo Bar"}, "s7")
	if !changed {
		t.Fatal("changed = false")
	}
	if string(got) != "Foo Bar ^s7\n===\n" {
		t.Errorf("got %q", got)
	}
	blocks, _ := parse(t, string(got))
	if blocks[0].Anchor != "s7" || blocks[0].Kind != KindHeading {
		t.Errorf("回填后解析 anchor/kind = %q/%v", blocks[0].Anchor, blocks[0].Kind)
	}
}

// TestAppendHeadingAnchor_Frontmatter frontmatter 原样保留，锚只进正文。
func TestAppendHeadingAnchor_Frontmatter(t *testing.T) {
	md := "---\ntitle: 测试\ntags: [a]\n---\n\n# Head\n\n体。\n"
	got, changed := AppendHeadingAnchor([]byte(md), []string{"Head"}, "f9")
	if !changed {
		t.Fatal("changed = false")
	}
	s := string(got)
	if !strings.HasPrefix(s, "---\ntitle: 测试\ntags: [a]\n---\n") {
		t.Errorf("frontmatter 被改写:\n%q", s)
	}
	if !strings.Contains(s, "# Head ^f9") {
		t.Errorf("锚未落到 heading:\n%q", s)
	}
}

// TestAppendHeadingAnchor_Miss 路径未命中 / 空路径 / 空锚 → false 且原文不动。
func TestAppendHeadingAnchor_Miss(t *testing.T) {
	md := "# Alpha\n\n体。\n"
	if got, changed := AppendHeadingAnchor([]byte(md), []string{"Nope"}, "x1"); changed || string(got) != md {
		t.Errorf("未命中应原文返回: changed=%v got=%q", changed, got)
	}
	if got, changed := AppendHeadingAnchor([]byte(md), nil, "x1"); changed || string(got) != md {
		t.Errorf("空路径应原文返回: changed=%v", changed)
	}
	if got, changed := AppendHeadingAnchor([]byte(md), []string{"Alpha"}, ""); changed || string(got) != md {
		t.Errorf("空锚应原文返回: changed=%v", changed)
	}
}

// TestAppendHeadingAnchor_HeadingOnly 路径只匹配 heading 块：段落内容
// 与路径文本相同也不命中。
func TestAppendHeadingAnchor_HeadingOnly(t *testing.T) {
	md := "Foo\n\n体。\n"
	if _, changed := AppendHeadingAnchor([]byte(md), []string{"Foo"}, "x1"); changed {
		t.Error("段落不应被锚（仅 heading 块可定位）")
	}
}
