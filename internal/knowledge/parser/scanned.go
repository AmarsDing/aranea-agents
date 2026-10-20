/*
	U2-3 解析纵深：疑似扫描页检测。

PDFToMarkdown 产物带 <!-- page:N --> 页锚（pdf.go）。图像型扫描页的文字层
通常只剩水印/页眉碎片或整页缺失，据此把「锚内正文字符数低于阈值」的页
判为疑似扫描，给出走视觉模型抽取的建议。纯新增，不改动 PDFToMarkdown。
*/
package parser

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// pageAnchorRe 匹配 PDFToMarkdown 产物的页锚注释行 <!-- page:N -->。
var pageAnchorRe = regexp.MustCompile(`(?m)^<!--\s*page:(\d+)\s*-->\s*$`)

// PageSpan 一段页锚区间（G-07 页码锚定的原料导出）。
type PageSpan struct {
	// Page 页码（PDFToMarkdown 物理页序，从 1 起）。
	Page int
	// ByteStart 页锚注释行在文本中的起始字节偏移；锚下一锚（或文末）之间的内容
	// 均归属本页——锚行之前的内容无页码归属（PageNo 缺省不落元数据）。
	ByteStart int
}

// PageAnchorSpans 提取 PDFToMarkdown 产物的页锚区间（按出现顺序升序）。
// 无锚输入返回 nil（非 PDF 链路产物，调用方据此跳过页码锚定）。
func PageAnchorSpans(text string) []PageSpan {
	locs := pageAnchorRe.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([]PageSpan, 0, len(locs))
	for _, loc := range locs {
		n, err := strconv.Atoi(text[loc[2]:loc[3]])
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, PageSpan{Page: n, ByteStart: loc[0]})
	}
	return out
}

// scannedBodyMinRunes 每页正文（剔除全部空白后）低于该字符数判疑似扫描。
const scannedBodyMinRunes = 20

// scannedSuggestion 对疑似扫描页的统一建议文案。
const scannedSuggestion = "该页可能为扫描图像，建议走视觉模型抽取"

// ScannedReport 疑似扫描页检测结果。
type ScannedReport struct {
	// Pages 疑似扫描的页码（按产物中出现顺序）。
	Pages []int
	// Suggestion 处置建议文案；无疑似页时为空。
	Suggestion string
}

// AnalyzeScannedPages 按 <!-- page:N --> 锚切分 PDF 解析产物，逐页统计正文
// 字符数，低于 scannedBodyMinRunes 的页判为疑似扫描。无锚输入不产出任何
// 判定（非 PDFToMarkdown 产物，无权断言）。
func AnalyzeScannedPages(md string) ScannedReport {
	locs := pageAnchorRe.FindAllStringSubmatchIndex(md, -1)
	if len(locs) == 0 {
		return ScannedReport{}
	}
	var pages []int
	for i, loc := range locs {
		n, err := strconv.Atoi(md[loc[2]:loc[3]])
		if err != nil {
			continue
		}
		body := md[loc[1]:]
		if i+1 < len(locs) {
			body = md[loc[1]:locs[i+1][0]]
		}
		// 正文 = 剔除全部空白（空格/换行/表格管道间的空白）后的字符数。
		if utf8.RuneCountInString(strings.Join(strings.Fields(body), "")) < scannedBodyMinRunes {
			pages = append(pages, n)
		}
	}
	if len(pages) == 0 {
		return ScannedReport{}
	}
	return ScannedReport{Pages: pages, Suggestion: scannedSuggestion}
}
