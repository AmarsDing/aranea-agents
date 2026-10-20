package knowledge

import (
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// G-07 锚索引：页锚按起点归并、锚前内容无归属、标题栈嵌套、code fence 假标题免疫。
func TestDocAnchors_PageAndHeading(t *testing.T) {
	text := strings.TrimSpace(`
# 部署手册

前置说明段落。

<!-- page:1 -->

## 环境准备

第一页正文。

<!-- page:2 -->

第二页正文，无新标题。
`)
	a := newDocAnchors(text)
	if a == nil {
		t.Fatal("anchors must be built from page-anchored markdown")
	}
	idxOf := func(needle string) int {
		i := strings.Index(text, needle)
		if i < 0 {
			t.Fatalf("fixture text missing %q", needle)
		}
		return i
	}
	got := a.anchorAt(idxOf("前置说明段落"))
	if got.PageNo != 0 || len(got.HeadingPath) != 1 || got.HeadingPath[0] != "部署手册" {
		t.Fatalf("pre-anchor chunk must carry heading only, got %+v", got)
	}
	got = a.anchorAt(idxOf("第一页正文"))
	if got.PageNo != 1 || len(got.HeadingPath) != 2 || got.HeadingPath[1] != "环境准备" {
		t.Fatalf("page-1 chunk anchor wrong: %+v", got)
	}
	got = a.anchorAt(idxOf("第二页正文"))
	if got.PageNo != 2 || len(got.HeadingPath) != 2 || got.HeadingPath[1] != "环境准备" {
		t.Fatalf("heading must carry across pages until next heading, got %+v", got)
	}
}

// code fence 内 # 行不得成为标题锚；无锚文本索引为空。
func TestDocAnchors_CodeFenceImmunityAndEmpty(t *testing.T) {
	text := "# 真标题\n\n```\n# 假标题\n```\n\n正文随后。\n"
	a := newDocAnchors(text)
	if a == nil {
		t.Fatal("real heading must be indexed")
	}
	got := a.anchorAt(strings.Index(text, "正文随后"))
	if len(got.HeadingPath) != 1 || got.HeadingPath[0] != "真标题" {
		t.Fatalf("fence content must not shadow real heading, got %+v", got)
	}
	got = a.anchorAt(strings.Index(text, "# 假标题"))
	if len(got.HeadingPath) != 1 || got.HeadingPath[0] != "真标题" {
		t.Fatalf("fence # line must not be a heading span, got %+v", got)
	}
	if newDocAnchors("plain text without anchors or headings") != nil {
		t.Fatal("anchorless text must yield nil index (fast path)")
	}
	// ByteStart=-1（trpc 定位失败）不取任何锚，宁缺不假。
	if got := a.anchorAt(-1); !got.empty() {
		t.Fatalf("unknown offset must be anchorless, got %+v", got)
	}
}

// metadata 归并：无锚原样不动（字节级稳定）；有锚注入键并保留文档级键、块级覆盖同名键。
func TestMergeChunkAnchorMetadata(t *testing.T) {
	if got := mergeChunkAnchorMetadata(`{"a":1}`, chunkAnchor{}); got != `{"a":1}` {
		t.Fatalf("anchorless merge must keep bytes, got %q", got)
	}
	got := mergeChunkAnchorMetadata(`{"a":1,"page_no":99}`, chunkAnchor{PageNo: 3, HeadingPath: []string{"章", "节"}})
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("merged metadata invalid: %v (%q)", err, got)
	}
	if string(m["a"]) != "1" || string(m["page_no"]) != "3" || string(m["heading_path"]) != `["章","节"]` {
		t.Fatalf("merged metadata wrong: %q", got)
	}
	// 纯标题锚：page_no 不落键。
	got = mergeChunkAnchorMetadata("{}", chunkAnchor{HeadingPath: []string{"章"}})
	if strings.Contains(got, "page_no") {
		t.Fatalf("page_no=0 must not be persisted: %q", got)
	}
}

// 全链：PDF 页锚文本 → BuildIndexedChunks（char 窗跨页）→ 行 metadata 带 page_no；
// 父子策略子块同样带锚（citation 锚定到页的原料）。锚按块起点归并——跨页窗起点
// 落前页即归前页（起点即引用起点，NotebookLM 式回跳口径），故 asserting 存在
// 完整落入第 2 页的块带 page_no=2，且存在带 page_no=1 的块。
func TestBuildIndexedChunks_AnchorsRoundTrip(t *testing.T) {
	pdf := "<!-- page:1 -->\n\n" + strings.Repeat("第一页正文段落。", 40) +
		"\n\n<!-- page:2 -->\n\n" + strings.Repeat("第二页正文段落。", 40)
	mk := func(strategy ChunkStrategy) {
		chunks, err := BuildIndexedChunks(t.Context(), nil, IngestParams{
			DocID: "doc1", CollectionID: "col1", Text: pdf,
			Strategy: strategy, ChunkSize: 40, ChunkOverlap: 8, ParentChunkSize: 120,
		}, nil)
		if err != nil {
			t.Fatalf("BuildIndexedChunks(%s): %v", strategy, err)
		}
		marker := "<!-- page:2 -->"
		// 起点锚：块起点 >= 页锚行起点才落 page 2（块可起在锚行内部，带上 --> 残段——
		// 故区间从锚行起点算而非行尾）。
		page2Region := pdf[strings.Index(pdf, marker):]
		pageOf := func(c biz.KnowledgeChunk) string {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(c.MetadataJSON), &m); err != nil {
				t.Fatalf("chunk metadata invalid: %v (%q)", err, c.MetadataJSON)
			}
			return string(m["page_no"])
		}
		var page1, page2 bool
		for _, c := range chunks {
			if c.IsParent {
				continue // 父块语境行不入召回面，不要求页码语义
			}
			switch pageOf(c) {
			case "1":
				page1 = true
			case "2":
				page2 = true
				// 起点落第二页的块正文必须全属第二页区间（start-anchor 语义护卫）。
				if !strings.Contains(page2Region, c.Content) {
					t.Fatalf("strategy %s: page-2 anchored chunk not in page-2 region: %q", strategy, c.Content)
				}
			}
		}
		if !page1 || !page2 {
			t.Fatalf("strategy %s: expected chunks anchored on both pages (p1=%v p2=%v)", strategy, page1, page2)
		}
	}
	mk(ChunkByChar)
	mk(ChunkByToken)
	mk(ChunkByParentChild)
}

// 对抗审查 F1/F4/F8 回归：代码 fence 内的同名标题行不得抢占锚位（锚取
// blockparse 真源字节偏移，非子串重定位）——fence 与真标题之间的段落块
// 保持原标题栈，真标题之后的块正确归属。
func TestDocAnchors_FencedDuplicateHeading(t *testing.T) {
	text := "# 手册\n\n```markdown\n## 安装\n```\n\n介绍段落。\n\n## 安装\n\n安装步骤正文。\n"
	a := newDocAnchors(text)
	if a == nil {
		t.Fatal("anchors must be built for fenced-duplicate doc")
	}
	if got := a.anchorAt(strings.Index(text, "介绍段落")); len(got.HeadingPath) != 1 || got.HeadingPath[0] != "手册" {
		t.Fatalf("paragraph between fence and real heading must keep [手册], got %+v", got)
	}
	got := a.anchorAt(strings.Index(text, "安装步骤正文"))
	if len(got.HeadingPath) != 2 || got.HeadingPath[1] != "安装" {
		t.Fatalf("chunk under real heading must carry [手册 安装], got %+v", got)
	}
	// 行内 prose 提及同样不抢锚：真实标题行才计。
	text2 := "# 手册\n\n参照 ## 安装 一章。\n\n## 安装\n\n正文。\n"
	a2 := newDocAnchors(text2)
	got = a2.anchorAt(strings.Index(text2, "一章"))
	if len(got.HeadingPath) != 1 || got.HeadingPath[0] != "手册" {
		t.Fatalf("inline mention must not pre-anchor heading, got %+v", got)
	}
}

// 对抗审查 F2 回归：CRLF 文档——多行 setext 标题（归一后 b.Text 含 \n，原文
// 为 \r\n）与页锚均须可锚（norm 坐标系 + crlf 表换算 original 块偏移）。
func TestDocAnchors_CRLFAnchors(t *testing.T) {
	text := "标题甲\r\n更多行\r\n=====\r\n\r\n甲下正文。\r\n\r\n<!-- page:2 -->\r\n\r\n第二页正文。\r\n"
	a := newDocAnchors(text)
	if a == nil {
		t.Fatal("CRLF doc with setext heading must be anchored")
	}
	got := a.anchorAt(strings.Index(text, "甲下正文"))
	if len(got.HeadingPath) != 1 || got.HeadingPath[0] != "标题甲\n更多行" {
		t.Fatalf("multi-line setext heading lost under CRLF: %+v", got)
	}
	got = a.anchorAt(strings.Index(text, "第二页正文"))
	if got.PageNo != 2 || len(got.HeadingPath) != 1 {
		t.Fatalf("page+heading anchor under CRLF wrong: %+v", got)
	}
}

// 对抗审查 F11 回归：anchorAt「起点 <= 块偏移」等号边界直测——起点恰落锚行
// 首字节的块归属该锚（前一字节仍归前区间），防止 >= 突变静默透传。
func TestDocAnchors_EqualityEdge(t *testing.T) {
	text := "<!-- page:1 -->\n\n甲段落。\n\n<!-- page:2 -->\n\n## 乙节\n\n乙段落。\n"
	a := newDocAnchors(text)
	if a == nil {
		t.Fatal("anchors must be built")
	}
	m1 := strings.Index(text, "<!-- page:1 -->")
	m2 := strings.Index(text, "<!-- page:2 -->")
	h2 := strings.Index(text, "## 乙节")
	if got := a.anchorAt(m1); got.PageNo != 1 {
		t.Fatalf("chunk starting at page:1 marker must be page 1, got %+v", got)
	}
	if got := a.anchorAt(m2); got.PageNo != 2 {
		t.Fatalf("equality edge: chunk starting at page:2 marker must be page 2, got %+v", got)
	}
	if got := a.anchorAt(m2 - 1); got.PageNo != 1 {
		t.Fatalf("one byte before marker must stay page 1, got %+v", got)
	}
	if got := a.anchorAt(h2); len(got.HeadingPath) != 1 || got.HeadingPath[0] != "乙节" {
		t.Fatalf("chunk starting at heading line must carry that heading, got %+v", got)
	}
	if got := a.anchorAt(h2 - 1); len(got.HeadingPath) != 0 {
		t.Fatalf("one byte before heading must have no heading, got %+v", got)
	}
}
