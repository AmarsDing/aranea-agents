// Package knowledge — G-07 页码/标题锚定：从（TrimSpace 基准的）分块源文本提取
// 页锚区间（PDFToMarkdown 产物 <!-- page:N -->）与 ATX 标题栈，按块 ByteStart
// 归并到 chunk metadata_json。页码指向原件物理页（RAGFlow/NotebookLM 式引用回跳
// 原料），heading_path 沿 blockparse 同口径（ATX/setext 标题、code fence 免疫、
// frontmatter 剥离——经其 parser 产出而非裸行扫描，防假标题）。
package knowledge

import (
	"encoding/json"
	"sort"
	"strings"

	"aranea-agents/internal/knowledge/blockparse"
	"aranea-agents/internal/knowledge/parser"
)

// chunkAnchor 单块锚点（page_no=0 且 heading_path 空 = 无锚，不落元数据键）。
type chunkAnchor struct {
	PageNo      int
	HeadingPath []string
}

func (a chunkAnchor) empty() bool { return a.PageNo <= 0 && len(a.HeadingPath) == 0 }

type pageSpan struct {
	start int
	page  int
}

type headingSpan struct {
	start int
	path  []string
}

// docAnchors 文档级锚索引（页锚与标题栈恒为 norm 坐标系——\r\n 归一为 \n 后
// 的文本；anchorAt 接收 original 坐标块偏移并经 crlf 表换算）。
type docAnchors struct {
	pages    []pageSpan
	headings []headingSpan
	// crlf 记录原文每个 \r\n 的 \r 字节偏移（升序），供 original→norm 坐标换算；
	// LF 文档为 nil（恒等）。span 起点恒为行首标记字节（</#/* 等），块首字节
	// 恰好落在 \r 上时按「起点即归属」划归前一区间。
	crlf []int
}

func (a *docAnchors) empty() bool {
	return a == nil || (len(a.pages) == 0 && len(a.headings) == 0)
}

// newDocAnchors 抽取页锚与标题路径。text 必须与分块入口同款 TrimSpace 基准
// （chunker/parent_child 均先 TrimSpace 再切窗，ByteStart 在此坐标系内）。
func newDocAnchors(text string) *docAnchors {
	a := &docAnchors{}
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	// 页锚：直接对 norm 取偏移（免 CRLF 换算）。
	for _, s := range parser.PageAnchorSpans(norm) {
		a.pages = append(a.pages, pageSpan{start: s.ByteStart, page: s.Page})
	}
	// 标题栈：blockparse 真源字节偏移（body 坐标系）+ frontmatter 前缀补偿归并
	// 到 norm 坐标系——不做子串重定位：代码 fence/行内 prose 中的同名 "## X"
	// 串不再抢占锚位（对抗审查 F1/F4/F8）；多行 setext 标题在 norm 坐标系内
	// 天然可锚（CRLF 失信 F2）。walk 文档序 = 偏移升序，无需再守卫。
	if blocks, _, parseErr := blockparse.Parse("", []byte(norm)); parseErr == nil && len(blocks) > 0 {
		skip := len(norm) - len(blockparse.BodyOf(norm))
		for _, b := range blocks {
			if b.Kind != blockparse.KindHeading || len(b.HeadingPath) == 0 || b.ByteStart < 0 {
				continue
			}
			a.headings = append(a.headings, headingSpan{start: skip + b.ByteStart, path: b.HeadingPath})
		}
	}
	if a.empty() {
		return nil
	}
	for i := 0; i+1 < len(text); i++ {
		if text[i] == '\r' && text[i+1] == '\n' {
			a.crlf = append(a.crlf, i)
		}
	}
	return a
}

// anchorAt 归并块锚：页锚为起点不大于块偏移的最后一锚（锚前内容 PageNo=0，
// 无归属不落键）；标题同理。byteStart<0（定位失败）时仅保留「全文之前」的头
// 部语境不取——返回无锚，宁缺不假。
func (a *docAnchors) anchorAt(byteStart int) chunkAnchor {
	if a.empty() || byteStart < 0 {
		return chunkAnchor{}
	}
	n := byteStart
	if len(a.crlf) > 0 {
		n = byteStart - sort.Search(len(a.crlf), func(i int) bool { return a.crlf[i] >= byteStart })
	}
	out := chunkAnchor{}
	for _, s := range a.pages {
		if s.start > n {
			break
		}
		out.PageNo = s.page
	}
	for _, s := range a.headings {
		if s.start > n {
			break
		}
		out.HeadingPath = s.path
	}
	return out
}

// mergeChunkAnchorMetadata 把块锚并回 chunk metadata_json（文档级元数据先行）。
// 键：page_no（int，>0 才有）、heading_path（[]string，块标题路径——不含页面
// 锚内层级，纯 markdown 标题栈）。无锚时原样返回 meta（字节级不动，保持存量
// 无锚文档落库口径与 cipher 摘要稳定）。文档级同键被块锚覆盖——块级定位更准。
func mergeChunkAnchorMetadata(meta string, a chunkAnchor) string {
	if a.empty() {
		return meta
	}
	m := map[string]json.RawMessage{}
	if trimmed := strings.TrimSpace(meta); trimmed != "" && strings.HasPrefix(trimmed, "{") {
		// NormalizeMetadataJSON 已保证合法 JSON；解析失败时不因锚定丢文档级元数据。
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return meta
		}
	}
	if a.PageNo > 0 {
		if raw, err := json.Marshal(a.PageNo); err == nil {
			m["page_no"] = raw
		}
	}
	if len(a.HeadingPath) > 0 {
		if raw, err := json.Marshal(a.HeadingPath); err == nil {
			m["heading_path"] = raw
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return meta
	}
	return string(out)
}
