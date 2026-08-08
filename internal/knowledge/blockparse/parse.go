// Package blockparse 双链块解析管线（SP1-A，设计 37-knowledge.design.md §S3）。
//
// 纯函数契约：Parse(docKey, markdown) (blocks, refs, err)——无 IO、无全局态。
// 实现路线：goldmark AST 负责块结构与 code span 排除，wikilink 扫描器在 AST
// 定位的连续文本 run 上扫描 `[[...]]`/`![[...]]`，保证源码位置准确（Context 用）。
package blockparse

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// EdgeType 引用边类型（refs.edge_type）。
type EdgeType string

const (
	EdgeRef   EdgeType = "ref"   // [[...]]
	EdgeEmbed EdgeType = "embed" // ![[...]]
)

// BlockKind 块类型（blocks.kind）。
type BlockKind string

const (
	KindHeading    BlockKind = "heading"
	KindParagraph  BlockKind = "paragraph"
	KindListItem   BlockKind = "list_item"
	KindCodeBlock  BlockKind = "code_block"
	KindBlockquote BlockKind = "blockquote"
	KindTable      BlockKind = "table"
	KindMath       BlockKind = "math"
)

// BlockRow 块行（knowledge_blocks 的解析产物）。
type BlockRow struct {
	Ordinal     int
	Kind        BlockKind
	Anchor      string   // 显式 ^anchor（无锚为空）
	HeadingPath []string // heading 块的标题路径（含自身）
	ContentHash string   // 锚点剥离后的规范化文本 hash（锚点回填不触发重算）
	TextExcerpt string   // 前 200 rune（反链上下文/图谱标签用）
}

// RefRow 引用边行（knowledge_block_refs 的解析产物）。
type RefRow struct {
	SrcOrdinal int      // 源块 ordinal
	RawTarget  string   // 原始目标文本（dangling 解析必需）
	Alias      string   // |alias（无别名/空别名为空）
	EdgeType   EdgeType // ref / embed
	Context    string   // 引用上下文 ±50 rune
}

// DocMeta 文档级元数据（frontmatter 受管字段中 Resolver 需要的子集）。
type DocMeta struct {
	Title   string
	Aliases []string
}

const (
	excerptRunes = 200
	contextRunes = 50
)

var (
	// 块尾锚：` ^abc` 结尾（^ 前须空白/行首，id 字符 [A-Za-z0-9_-]）。
	anchorSuffixRe = regexp.MustCompile(`(?:^|\s)\^([A-Za-z0-9_-]+)$`)
	// 独立锚行：整块文本仅 `^abc`（附着前一块，自身不成块）。
	anchorSoloRe = regexp.MustCompile(`^\^([A-Za-z0-9_-]+)$`)
)

// Parse 解析 markdown 为块行 + 引用边行。docKey 仅为契约占位（行内不落 doc 标识，
// doc 归属由调用方在落库时补充）。err 恒为 nil（容错解析，永不失败）。
func Parse(_ string, src []byte) ([]BlockRow, []RefRow, error) {
	body, _ := splitFrontmatter(normalizeNewlines(string(src)))
	if strings.TrimSpace(body) == "" {
		return nil, nil, nil
	}
	p := &parser{
		src:   body,
		runes: []rune(body),
		doc:   goldmarkParser().Parse(text.NewReader([]byte(body))),
	}
	p.walk()
	return p.blocks, p.refs, nil
}

// ParseDocMeta 提取 frontmatter 的 title/aliases。无 frontmatter 或 YAML 损坏返回零值
// （损坏口径与 biz parseVaultDoc 一致：整篇按正文）。
func ParseDocMeta(src []byte) DocMeta {
	_, meta, ok := splitFrontmatterKV(normalizeNewlines(string(src)))
	if !ok {
		return DocMeta{}
	}
	var fm struct {
		Title   string   `yaml:"title"`
		Aliases []string `yaml:"aliases"`
	}
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return DocMeta{}
	}
	return DocMeta{Title: fm.Title, Aliases: fm.Aliases}
}

// parser 一次解析的会话状态（Parse 内创建，非共享）。
type parser struct {
	src   string
	runes []rune // src 的 rune 视图（Context ±50 rune 窗口用）
	doc   ast.Node

	blocks []BlockRow
	refs   []RefRow

	headings []headingEntry // 标题栈（heading_path 推导）
}

type headingEntry struct {
	level int
	text  string
}

func (p *parser) walk() {
	for n := p.doc.FirstChild(); n != nil; n = n.NextSibling() {
		switch n.Kind() {
		case ast.KindHeading:
			p.emitHeading(n.(*ast.Heading))
		case ast.KindParagraph:
			p.emitParagraph(n)
		case ast.KindList:
			p.walkList(n)
		case ast.KindBlockquote:
			p.emitRich(KindBlockquote, n)
		case extast.KindTable:
			p.emitRich(KindTable, n)
		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			p.emitCode(n)
		default:
			// ThematicBreak / HTMLBlock 等不成块。
		}
	}
}

// walkList 列表递归：每个 ListItem 独立成块（先父后子，pre-order）。
func (p *parser) walkList(list ast.Node) {
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.Kind() != ast.KindListItem {
			continue
		}
		// 自身文本 = 首个块子节点（tight 列表为 TextBlock）；嵌套列表不计入。
		var own ast.Node
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			if c.Kind() == ast.KindParagraph || c.Kind() == ast.KindTextBlock {
				own = c
				break
			}
		}
		if own != nil {
			p.emitRich(KindListItem, own)
		}
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			if c.Kind() == ast.KindList {
				p.walkList(c)
			}
		}
	}
}

func (p *parser) emitHeading(h *ast.Heading) {
	runs := collectRuns(h, p.src)
	plain, anchor := splitAnchor(joinRuns(runs, p.src))
	// 标题栈：同级/更深层弹出后压入。
	for len(p.headings) > 0 && p.headings[len(p.headings)-1].level >= h.Level {
		p.headings = p.headings[:len(p.headings)-1]
	}
	p.headings = append(p.headings, headingEntry{level: h.Level, text: plain})
	path := make([]string, 0, len(p.headings))
	for _, e := range p.headings {
		path = append(path, e.text)
	}
	p.emit(KindHeading, runs, plain, anchor, path)
}

func (p *parser) emitParagraph(n ast.Node) {
	runs := collectRuns(n, p.src)
	raw := joinRuns(runs, p.src)
	// 独立锚行：附着前一块，自身不成块。
	if id, ok := matchAnchor(anchorSoloRe, strings.TrimSpace(raw)); ok {
		if len(p.blocks) > 0 && p.blocks[len(p.blocks)-1].Anchor == "" {
			p.blocks[len(p.blocks)-1].Anchor = id
		}
		return
	}
	plain, anchor := splitAnchor(raw)
	kind := KindParagraph
	if isMathBlock(plain) {
		kind = KindMath
	}
	p.emit(kind, runs, plain, anchor, p.currentPath())
}

// emitRich 引用块/表格/列表项：runs 取自节点全部文本后代（CodeSpan 子树除外）。
func (p *parser) emitRich(kind BlockKind, n ast.Node) {
	runs := collectRuns(n, p.src)
	plain, anchor := splitAnchor(joinRuns(runs, p.src))
	p.emit(kind, runs, plain, anchor, p.currentPath())
}

// emitCode 代码块：成块但不扫描引用（代码内容无 wikilink 语义）。
func (p *parser) emitCode(n ast.Node) {
	var sb strings.Builder
	block := n.(interface{ Lines() *text.Segments })
	lines := block.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.WriteString(string(seg.Value([]byte(p.src))))
	}
	plain, anchor := splitAnchor(strings.TrimRight(sb.String(), "\n"))
	p.emit(KindCodeBlock, nil, plain, anchor, p.currentPath())
}

// emit 统一成块入口：plain 已剥离锚点；runs 为引用扫描范围（nil 表示不扫描）。
func (p *parser) emit(kind BlockKind, runs []text.Segment, plain, anchor string, path []string) {
	normalized := normalizeSpace(plain)
	b := BlockRow{
		Ordinal:     len(p.blocks),
		Kind:        kind,
		Anchor:      anchor,
		HeadingPath: path,
		ContentHash: hashText(normalized),
		TextExcerpt: truncateRunes(wikilinkDisplay(normalized), excerptRunes),
	}
	p.blocks = append(p.blocks, b)
	p.scanRefs(b.Ordinal, runs)
}

func (p *parser) currentPath() []string {
	if len(p.headings) == 0 {
		return nil
	}
	path := make([]string, 0, len(p.headings))
	for _, e := range p.headings {
		path = append(path, e.text)
	}
	return path
}

// scanRefs 在文本 run 上扫描 wikilink。run 由 AST 文本节点合并而来，
// CodeSpan 子树已被排除，故行内代码天然不产生引用。
func (p *parser) scanRefs(ordinal int, runs []text.Segment) {
	for _, run := range runs {
		s := string(run.Value([]byte(p.src)))
		base := run.Start
		i := 0
		for i < len(s) {
			j := strings.Index(s[i:], "[[")
			if j < 0 {
				break
			}
			start := i + j // '[[' 起始（run 内偏移）
			edge := EdgeRef
			contentStart := start + 2
			if start > 0 && s[start-1] == '!' {
				edge = EdgeEmbed
			}
			end := strings.Index(s[contentStart:], "]]")
			if end < 0 {
				break // 未闭合：其后不会再有新 '[[' 配对成功之外的语义，直接停扫本 run
			}
			content := s[contentStart : contentStart+end]
			next := contentStart + end + 2
			if validLinkContent(content) {
				if ref, ok := buildRef(content, edge); ok {
					ref.SrcOrdinal = ordinal
					ref.Context = p.contextWindow(base+start, base+next)
					p.refs = append(p.refs, ref)
				}
			}
			i = next
		}
	}
}

// validLinkContent 目标段合法性：不含方括号与换行（`[[a]b]]`、跨行不闭合均为非法）。
func validLinkContent(content string) bool {
	return !strings.ContainsAny(content, "[]\n\r")
}

// buildRef 切分 target|alias：首个 | 前为目标（trim），其余整体为别名（trim；
// 空别名等价无别名；别名可含 |）。目标 trim 后为空则非法。
func buildRef(content string, edge EdgeType) (RefRow, bool) {
	target, alias, _ := strings.Cut(content, "|")
	target = strings.TrimSpace(target)
	alias = strings.TrimSpace(alias)
	if target == "" {
		return RefRow{}, false
	}
	return RefRow{RawTarget: target, Alias: alias, EdgeType: edge}, true
}

// contextWindow 源文本 ±50 rune 窗口（UTF-8 安全）。start/stop 为字节偏移。
func (p *parser) contextWindow(start, stop int) string {
	lo := utf8.RuneCountInString(p.src[:start]) - contextRunes
	if lo < 0 {
		lo = 0
	}
	hi := utf8.RuneCountInString(p.src[:stop]) + contextRunes
	if hi > len(p.runes) {
		hi = len(p.runes)
	}
	return string(p.runes[lo:hi])
}

// collectRuns 收集节点全部文本后代的源码段，合并相邻段为连续 run；
// CodeSpan 子树整体跳过（行内代码不扫描、不进摘录）。
func collectRuns(n ast.Node, src string) []text.Segment {
	var segs []text.Segment
	var walk func(m ast.Node)
	walk = func(m ast.Node) {
		if m.Kind() == ast.KindCodeSpan {
			return
		}
		if t, ok := m.(*ast.Text); ok {
			segs = append(segs, t.Segment)
		}
		for c := m.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(n)
	return mergeRuns(segs, src)
}

func mergeRuns(segs []text.Segment, _ string) []text.Segment {
	if len(segs) == 0 {
		return nil
	}
	runs := []text.Segment{segs[0]}
	for _, s := range segs[1:] {
		last := &runs[len(runs)-1]
		if s.Start <= last.Stop { // 相邻/重叠：合并
			if s.Stop > last.Stop {
				runs[len(runs)-1] = text.NewSegment(last.Start, s.Stop)
			}
			continue
		}
		runs = append(runs, s)
	}
	return runs
}

// joinRuns 拼接 run 文本：段间空隙含换行补 \n，否则补空格（表格单元/标记间隙）。
func joinRuns(runs []text.Segment, src string) string {
	var sb strings.Builder
	prevStop := -1
	for _, r := range runs {
		if prevStop >= 0 && r.Start > prevStop {
			if strings.Contains(src[prevStop:r.Start], "\n") {
				sb.WriteByte('\n')
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(string(r.Value([]byte(src))))
		prevStop = r.Stop
	}
	return sb.String()
}

// splitAnchor 剥离块尾显式锚，返回（净文本, 锚 id）。
func splitAnchor(plain string) (string, string) {
	trimmed := strings.TrimSpace(plain)
	loc := anchorSuffixRe.FindStringSubmatchIndex(trimmed)
	if loc == nil {
		return plain, ""
	}
	return strings.TrimSpace(trimmed[:loc[0]]), trimmed[loc[2]:loc[3]]
}

func matchAnchor(re *regexp.Regexp, s string) (string, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// isMathBlock 最小启发式：整块以 $$ 起止。
func isMathBlock(plain string) bool {
	t := strings.TrimSpace(plain)
	return len(t) >= 4 && strings.HasPrefix(t, "$$") && strings.HasSuffix(t, "$$")
}

// wikilinkDisplay 摘录中的 wikilink 替换为显示文本（别名优先，否则目标）。
func wikilinkDisplay(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], "[[")
		if j < 0 {
			sb.WriteString(s[i:])
			break
		}
		start := i + j
		end := strings.Index(s[start+2:], "]]")
		if end < 0 {
			sb.WriteString(s[i:])
			break
		}
		content := s[start+2 : start+2+end]
		if !validLinkContent(content) {
			sb.WriteString(s[i : start+2+end+2])
			i = start + 2 + end + 2
			continue
		}
		target, alias, _ := strings.Cut(content, "|")
		display := strings.TrimSpace(alias)
		if display == "" {
			display = strings.TrimSpace(target)
		}
		head := s[i:start]
		// 嵌入语法的前导 ! 一并剔除。
		if start > i && strings.HasSuffix(head, "!") {
			head = head[:len(head)-1]
		}
		sb.WriteString(head)
		sb.WriteString(display)
		i = start + 2 + end + 2
	}
	return sb.String()
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// splitFrontmatter 剥离合法 frontmatter（首行 --- 起、独立 --- 行止、YAML 可解析）。
// 任何一步不满足即视为无 frontmatter（整篇按正文，与 biz parseVaultDoc 口径一致）。
func splitFrontmatter(src string) (body string, ok bool) {
	body, meta, ok := splitFrontmatterKV(src)
	if !ok {
		return src, false
	}
	var probe any
	if err := yaml.Unmarshal([]byte(meta), &probe); err != nil {
		return src, false
	}
	return body, true
}

// splitFrontmatterKV 仅按分隔行切分 frontmatter（不校验 YAML），返回 body 与原始 meta。
func splitFrontmatterKV(src string) (body, meta string, ok bool) {
	if !strings.HasPrefix(src, "---\n") {
		return src, "", false
	}
	rest := src[len("---\n"):]
	off := 0
	for off <= len(rest) {
		line := rest[off:]
		nl := strings.IndexByte(line, '\n')
		var content string
		if nl < 0 {
			content = line
		} else {
			content = line[:nl]
		}
		if content == "---" {
			meta = rest[:off]
			if nl < 0 {
				return "", meta, true
			}
			return rest[off+nl+1:], meta, true
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	return src, "", false
}

// goldmarkParser 构造启用表格扩展的解析器（每次新建：Parser 内部 Context 非复用安全）。
func goldmarkParser() gmparser.Parser {
	return goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()
}
