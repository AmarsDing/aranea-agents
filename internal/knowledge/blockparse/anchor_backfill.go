package blockparse

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// AppendHeadingAnchor 惰性锚点回填纯函数（SP1-H，设计 S2/F-SP1-10）：
// 向匹配 headingPath 的首个 heading 块文本末尾追加 " ^anchor"。
//
// 口径与 Resolver/存储层一致（「取首」确定性）：
//   - 仅 heading 块可定位（段落同路径文本不命中）；
//   - 重复标题锚首个命中块；首个已锚时整体跳过（不顺延到后续同名块）。
//
// 幂等：已锚块返回 (src, false)；未命中/空路径/空锚同样原文返回。
// frontmatter 原样保留（锚只落正文）。变更时返回的文本统一为 LF 换行
// （与 Parse 归一化口径一致）；未变更时原样返回 src（不改换行风格）。
// 锚插在 ATX 闭合符之前（`## Foo ##` → `## Foo ^a ##`），保证重新解析可识别。
func AppendHeadingAnchor(src []byte, headingPath []string, anchor string) ([]byte, bool) {
	if len(headingPath) == 0 || anchor == "" {
		return src, false
	}
	norm := normalizeNewlines(string(src))
	body, _ := splitFrontmatter(norm)
	fmLen := len(norm) - len(body) // body 为 norm 后缀（frontmatter 前缀原样保留）

	doc := goldmarkParser().Parse(text.NewReader([]byte(body)))
	var headings []headingEntry
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		runs := collectRuns(h, body)
		plain, existing := splitAnchor(joinRuns(runs, body))
		for len(headings) > 0 && headings[len(headings)-1].level >= h.Level {
			headings = headings[:len(headings)-1]
		}
		headings = append(headings, headingEntry{level: h.Level, text: plain})
		if !pathStringsEqual(headings, headingPath) {
			continue
		}
		// 首个命中块：已锚或无文本可附（空 heading）→ 整体跳过。
		if existing != "" || len(runs) == 0 {
			return src, false
		}
		off := fmLen + runs[len(runs)-1].Stop
		out := norm[:off] + " ^" + anchor + norm[off:]
		return []byte(out), true
	}
	return src, false
}

// pathStringsEqual heading 栈与目标路径逐段相等（栈即当前 heading 的完整路径）。
func pathStringsEqual(stack []headingEntry, path []string) bool {
	if len(stack) != len(path) {
		return false
	}
	for i := range stack {
		if stack[i].text != path[i] {
			return false
		}
	}
	return true
}
