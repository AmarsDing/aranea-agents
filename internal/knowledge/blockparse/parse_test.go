package blockparse

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// refLite 表驱动断言用的轻量引用行。
type refLite struct {
	srcOrdinal int
	rawTarget  string
	alias      string
	edgeType   EdgeType
}

func parse(t *testing.T, md string) ([]BlockRow, []RefRow) {
	t.Helper()
	blocks, refs, err := Parse("test.md", []byte(md))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return blocks, refs
}

func toLite(refs []RefRow) []refLite {
	if len(refs) == 0 {
		return nil
	}
	out := make([]refLite, 0, len(refs))
	for _, r := range refs {
		out = append(out, refLite{r.SrcOrdinal, r.RawTarget, r.Alias, r.EdgeType})
	}
	return out
}

func assertRefs(t *testing.T, refs []RefRow, want []refLite) {
	t.Helper()
	got := toLite(refs)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refs = %+v, want %+v", got, want)
	}
}

func blockKinds(blocks []BlockRow) []BlockKind {
	out := make([]BlockKind, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.Kind)
	}
	return out
}

// TestParseSyntaxMatrix 语法矩阵全覆盖（NFR-SP1-1）。
func TestParseSyntaxMatrix(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want []refLite
	}{
		{"基本引用", "见 [[Note]]。", []refLite{{0, "Note", "", EdgeRef}}},
		{"带别名", "见 [[Note|显示名]]。", []refLite{{0, "Note", "显示名", EdgeRef}}},
		{"标题引用", "见 [[Note#小节]]。", []refLite{{0, "Note#小节", "", EdgeRef}}},
		{"块锚引用", "见 [[Note#^blk1]]。", []refLite{{0, "Note#^blk1", "", EdgeRef}}},
		{"多级标题路径", "见 [[Note#H1#H2]]。", []refLite{{0, "Note#H1#H2", "", EdgeRef}}},
		{"同文档标题", "见 [[#小节]]。", []refLite{{0, "#小节", "", EdgeRef}}},
		{"同文档锚点", "见 [[#^a1]]。", []refLite{{0, "#^a1", "", EdgeRef}}},
		{"嵌入", "如下 ![[img.png]] 完。", []refLite{{0, "img.png", "", EdgeEmbed}}},
		{"嵌入带尺寸", "![[img.png|300]]", []refLite{{0, "img.png", "300", EdgeEmbed}}},
		{"目标两侧空白", "[[  Note  ]]", []refLite{{0, "Note", "", EdgeRef}}},
		{"单块多链接", "[[A]] 和 [[B]]", []refLite{{0, "A", "", EdgeRef}, {0, "B", "", EdgeRef}}},
		{"代码span不算", "`[[x]]`", nil},
		{"代码块不算", "```\n[[x]]\n```", nil},
		{"行内代码混合", "前 `[[no]]` 后 [[yes]]", []refLite{{0, "yes", "", EdgeRef}}},
		{"标准md链接不算", "[文本](https://example.com)", nil},
		{"图片链接不算", "![alt](img.png)", nil},
		{"未闭合", "[[foo", nil},
		{"空目标", "[[]]", nil},
		{"纯空白目标", "[[   ]]", nil},
		{"空别名等价无别名", "[[a|]]", []refLite{{0, "a", "", EdgeRef}}},
		{"别名含竖线", "[[a|b|c]]", []refLite{{0, "a", "b|c", EdgeRef}}},
		{"单右括号不闭合", "[[a]b]]", nil},
		{"跨行不闭合", "[[a\nb]]", nil},
		{"标题内引用", "# 见 [[X]]\n", []refLite{{0, "X", "", EdgeRef}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, refs := parse(t, tc.md)
			assertRefs(t, refs, tc.want)
		})
	}
}

// TestParseBlocks 块切分：kind/ordinal/heading_path。
func TestParseBlocks(t *testing.T) {
	md := `# Alpha

第一段正文。

## Beta

第二段有 [[Target Doc]] 链接。

# Gamma
`
	blocks, refs := parse(t, md)
	wantKinds := []BlockKind{KindHeading, KindParagraph, KindHeading, KindParagraph, KindHeading}
	if !reflect.DeepEqual(blockKinds(blocks), wantKinds) {
		t.Fatalf("kinds = %v, want %v", blockKinds(blocks), wantKinds)
	}
	for i, b := range blocks {
		if b.Ordinal != i {
			t.Errorf("blocks[%d].Ordinal = %d", i, b.Ordinal)
		}
	}
	if !reflect.DeepEqual(blocks[0].HeadingPath, []string{"Alpha"}) {
		t.Errorf("blocks[0].HeadingPath = %v", blocks[0].HeadingPath)
	}
	if !reflect.DeepEqual(blocks[2].HeadingPath, []string{"Alpha", "Beta"}) {
		t.Errorf("blocks[2].HeadingPath = %v", blocks[2].HeadingPath)
	}
	if !reflect.DeepEqual(blocks[4].HeadingPath, []string{"Gamma"}) {
		t.Errorf("blocks[4].HeadingPath = %v", blocks[4].HeadingPath)
	}
	if got := blocks[3].TextExcerpt; got != "第二段有 Target Doc 链接。" {
		t.Errorf("blocks[3].TextExcerpt = %q", got)
	}
	assertRefs(t, refs, []refLite{{3, "Target Doc", "", EdgeRef}})
}

// TestParseAnchors 显式锚点：行尾锚 / 标题锚 / 独立锚行 / 非锚 caret。
func TestParseAnchors(t *testing.T) {
	md := `# 标题 ^h0

第一段带锚。 ^p1

第二段。

^solo9

第三段。
`
	blocks, _ := parse(t, md)
	if len(blocks) != 4 {
		t.Fatalf("len(blocks) = %d, want 4（独立锚行不成块）: %+v", len(blocks), blockKinds(blocks))
	}
	if blocks[0].Anchor != "h0" || blocks[0].HeadingPath[0] != "标题" {
		t.Errorf("heading anchor/path = %q/%v", blocks[0].Anchor, blocks[0].HeadingPath)
	}
	if blocks[1].Anchor != "p1" || blocks[1].TextExcerpt != "第一段带锚。" {
		t.Errorf("para anchor/excerpt = %q/%q", blocks[1].Anchor, blocks[1].TextExcerpt)
	}
	if blocks[2].Anchor != "solo9" {
		t.Errorf("standalone anchor 应附着前一块，got %q", blocks[2].Anchor)
	}
	if blocks[3].Anchor != "" {
		t.Errorf("第三段应无锚，got %q", blocks[3].Anchor)
	}

	// 非锚 caret：中部 ^、无空格前缀的 ^2 均不视为锚点。
	b2, _ := parse(t, "价格 ^ 上涨\n\n公式 a^2 结尾")
	for i, b := range b2 {
		if b.Anchor != "" {
			t.Errorf("非锚 caret 误判: blocks[%d].Anchor = %q", i, b.Anchor)
		}
	}
}

// TestParseAnchorHashStable 锚点回填不改变内容 hash（回填不触发重算 embedding）。
func TestParseAnchorHashStable(t *testing.T) {
	b1, _ := parse(t, "完全相同的内容。")
	b2, _ := parse(t, "完全相同的内容。 ^abc")
	if b1[0].ContentHash != b2[0].ContentHash {
		t.Errorf("锚点改变 hash: %q vs %q", b1[0].ContentHash, b2[0].ContentHash)
	}
	if b2[0].Anchor != "abc" {
		t.Errorf("Anchor = %q", b2[0].Anchor)
	}
}

// TestParseFrontmatter frontmatter 不切块；title/aliases 供 Resolver。
func TestParseFrontmatter(t *testing.T) {
	md := "---\ntitle: 我的笔记\naliases: [笔记A, 别名B]\ntags: [x]\n---\n# 标题\n\n正文。"
	blocks, _ := parse(t, md)
	if !reflect.DeepEqual(blockKinds(blocks), []BlockKind{KindHeading, KindParagraph}) {
		t.Fatalf("frontmatter 被切块: %v", blockKinds(blocks))
	}
	meta := ParseDocMeta([]byte(md))
	if meta.Title != "我的笔记" || !reflect.DeepEqual(meta.Aliases, []string{"笔记A", "别名B"}) {
		t.Errorf("DocMeta = %+v", meta)
	}

	// CRLF 变体
	blocks2, _ := parse(t, "---\r\ntitle: T\r\n---\r\n# H")
	if len(blocks2) != 1 || blocks2[0].Kind != KindHeading {
		t.Errorf("CRLF frontmatter 处理错误: %v", blockKinds(blocks2))
	}
	// 无 frontmatter
	if m := ParseDocMeta([]byte("# 只有标题")); m.Title != "" || m.Aliases != nil {
		t.Errorf("无 frontmatter DocMeta 应为零值, got %+v", m)
	}
	// 损坏 frontmatter 整篇按正文（与 biz parseVaultDoc 口径一致）
	broken := "---\n: bad: yaml: [\n---\n正文"
	if m := ParseDocMeta([]byte(broken)); m.Title != "" {
		t.Errorf("损坏 frontmatter 应返回零值, got %+v", m)
	}
}

// TestParseNestedList 嵌套列表：每项独立成块，父项文本不含子项内容。
func TestParseNestedList(t *testing.T) {
	md := "- 父项 [[P]]\n  - 子项 [[C]]\n- 二项\n"
	blocks, refs := parse(t, md)
	wantKinds := []BlockKind{KindListItem, KindListItem, KindListItem}
	if !reflect.DeepEqual(blockKinds(blocks), wantKinds) {
		t.Fatalf("kinds = %v, want %v", blockKinds(blocks), wantKinds)
	}
	if strings.Contains(blocks[0].TextExcerpt, "子项") {
		t.Errorf("父项文本混入子项: %q", blocks[0].TextExcerpt)
	}
	assertRefs(t, refs, []refLite{{0, "P", "", EdgeRef}, {1, "C", "", EdgeRef}})
}

// TestParseBlockquoteAndTable 引用块/表格成块，内部链接归属正确。
func TestParseBlockquoteAndTable(t *testing.T) {
	md := "> 引用里有 [[Q]]。\n\n| a | b |\n|---|---|\n| [[T]] | 2 |\n"
	blocks, refs := parse(t, md)
	wantKinds := []BlockKind{KindBlockquote, KindTable}
	if !reflect.DeepEqual(blockKinds(blocks), wantKinds) {
		t.Fatalf("kinds = %v, want %v", blockKinds(blocks), wantKinds)
	}
	assertRefs(t, refs, []refLite{{0, "Q", "", EdgeRef}, {1, "T", "", EdgeRef}})
}

// TestParseMath $$ 段落归类为 math 块（最小启发式）。
func TestParseMath(t *testing.T) {
	blocks, _ := parse(t, "$$\nE=mc^2\n$$")
	if len(blocks) != 1 || blocks[0].Kind != KindMath {
		t.Fatalf("math 块识别失败: %v", blockKinds(blocks))
	}
	if blocks[0].Anchor != "" {
		t.Errorf("公式内 ^2 不应误判为锚点: %q", blocks[0].Anchor)
	}
	// 普通段落不受影响
	b2, _ := parse(t, "普通 $$未闭合")
	if b2[0].Kind != KindParagraph {
		t.Errorf("非 math 段落误判: %v", b2[0].Kind)
	}
}

// TestParseExcerptTruncation TextExcerpt 截断至 200 rune。
func TestParseExcerptTruncation(t *testing.T) {
	blocks, _ := parse(t, strings.Repeat("文", 250))
	if got := utf8.RuneCountInString(blocks[0].TextExcerpt); got != 200 {
		t.Errorf("excerpt runes = %d, want 200", got)
	}
}

// TestParseBlockText Text 为块的 Markdown 原文片段（SP1-G 晋升全文提取）：
// 可直接追加到目标文档重新解析出等价块。heading 带 ##、list_item 带 marker、
// blockquote 带 >、code_block 带 fence；显式锚保留在原文中。
func TestParseBlockText(t *testing.T) {
	md := "# 标题一\n\n第一段带 [[Target Doc]] 链接。 ^p1\n\n- 列表项甲\n- 列表项乙\n\n> 引用块\n\n```go\nfmt.Println()\n```\n"
	blocks, _ := parse(t, md)
	want := []string{
		"# 标题一",
		"第一段带 [[Target Doc]] 链接。 ^p1",
		"- 列表项甲",
		"- 列表项乙",
		"> 引用块",
		"```go\nfmt.Println()\n```",
	}
	if len(blocks) != len(want) {
		t.Fatalf("blocks = %d, want %d", len(blocks), len(want))
	}
	for i, w := range want {
		if blocks[i].Text != w {
			t.Errorf("blocks[%d].Text = %q, want %q", i, blocks[i].Text, w)
		}
	}
}

// TestParseBlockTextRoundTrip 晋升语义：块 Text 追加拼接后重解析，产出
// 同 kind 同内容（content_hash 一致）的块序列。
func TestParseBlockTextRoundTrip(t *testing.T) {
	md := "# H\n\npara one ^a\n\n- item x\n\n> quote\n\n```\ncode\n```\n"
	blocks, _ := parse(t, md)
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.Text)
	}
	rebuilt, _ := parse(t, strings.Join(texts, "\n\n")+"\n")
	if len(rebuilt) != len(blocks) {
		t.Fatalf("rebuilt = %d, want %d", len(rebuilt), len(blocks))
	}
	for i := range blocks {
		if rebuilt[i].Kind != blocks[i].Kind || rebuilt[i].ContentHash != blocks[i].ContentHash {
			t.Errorf("rebuilt[%d] = %v/%s, want %v/%s",
				i, rebuilt[i].Kind, rebuilt[i].ContentHash, blocks[i].Kind, blocks[i].ContentHash)
		}
		if rebuilt[i].Anchor != blocks[i].Anchor {
			t.Errorf("rebuilt[%d].Anchor = %q, want %q", i, rebuilt[i].Anchor, blocks[i].Anchor)
		}
	}
}

// TestParseContext Context 为链接前后 ±50 rune 的源文本（UTF-8 安全）。
func TestParseContext(t *testing.T) {
	pad := strings.Repeat("文", 60)
	md := pad + " [[目标]] " + pad
	_, refs := parse(t, md)
	if len(refs) != 1 {
		t.Fatalf("refs = %d", len(refs))
	}
	want := strings.Repeat("文", 49) + " [[目标]] " + strings.Repeat("文", 49)
	if refs[0].Context != want {
		t.Errorf("Context = %q, want %q", refs[0].Context, want)
	}
	if !utf8.ValidString(refs[0].Context) {
		t.Error("Context 非法 UTF-8")
	}
}

// TestParseDeterminism 纯函数确定性：同输入同输出。
func TestParseDeterminism(t *testing.T) {
	md := "---\ntitle: T\n---\n# A ^x\n\n段 [[B|c]] 落。\n\n- 项 ^y\n\n$$\nE\n$$\n"
	b1, r1 := parse(t, md)
	b2, r2 := parse(t, md)
	if !reflect.DeepEqual(b1, b2) || !reflect.DeepEqual(r1, r2) {
		t.Error("两次解析结果不一致")
	}
}

// TestParseEdgeCases 空文档/仅 frontmatter/分隔线。
func TestParseEdgeCases(t *testing.T) {
	b, r := parse(t, "")
	if len(b) != 0 || len(r) != 0 {
		t.Errorf("空文档: blocks=%d refs=%d", len(b), len(r))
	}
	b, _ = parse(t, "---\ntitle: x\n---\n")
	if len(b) != 0 {
		t.Errorf("仅 frontmatter: blocks=%d", len(b))
	}
	b, _ = parse(t, "# A\n\n---\n\nB")
	if !reflect.DeepEqual(blockKinds(b), []BlockKind{KindHeading, KindParagraph}) {
		t.Errorf("thematic break 应忽略: %v", blockKinds(b))
	}
}

// TestParseCRLF Windows 换行兼容。
func TestParseCRLF(t *testing.T) {
	blocks, refs := parse(t, "# T\r\n\r\n段落 [[A]]\r\n")
	if !reflect.DeepEqual(blockKinds(blocks), []BlockKind{KindHeading, KindParagraph}) {
		t.Fatalf("CRLF blocks = %v", blockKinds(blocks))
	}
	assertRefs(t, refs, []refLite{{1, "A", "", EdgeRef}})
}
