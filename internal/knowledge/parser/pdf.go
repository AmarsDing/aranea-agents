// Package parser 深度解析层（G-05 起步）：PDF 版面感知抽取，把 trpc reader
// 的纯文本拼接升级为结构化 Markdown——行簇重构（Content() 逐字坐标）、几何
// 列缝检测表格 → Markdown 表格、页码锚点注释 <!-- page:N -->（G-07 页码锚定
// 的原料）。无表格内容保持逐行平铺，绝不丢文本。
//
// 为什么不走 ledongthuc GetTextByRow：其 walkTextBlocks 不跟踪 Td 相对定位
// （fpdf/LibreOffice 系产物坐标全 0），且 Text.W/FontSize 不填。Content()
// 有完整 gstate（BT/Td/Tm/Tf/CTM）与逐字宽，是唯一可靠几何来源。
//
// sidecar 隔离：完全独立实现，不改动 trpc reader；上层在错误或零文本时回退
// 旧链路（document_extract.go extractOfficeText）。
package parser

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// 几何阈值全部相对页面字符中位宽 cellW（逐字真实宽，非字体猜测）。
const (
	// lineYClusterFactor：同基线行聚类容差 = cellW * factor（行内上下标偏移）。
	lineYClusterFactor = 0.6
	// colGapMinFactor：间隙超过 cellW*factor 视为新列起点（拉丁词间距 ≈0.5 宽）。
	colGapMinFactor = 1.45
	// colAlignTolerance：块内各行第 2 列起点 X 对齐容差（pt，绝对值——打印
	// 误差/字宽抖动最高 ~6pt）。
	colAlignTolerance = 9.0
	// minTableRows：认定表格的最少连续多列行数（含表头行）。
	minTableRows = 3
	// paragraphGapFactor：行距 > 中位行距 * factor 视作段落分界（补空行）。
	paragraphGapFactor = 1.85
)

// PDFToMarkdown 把 PDF 原始字节解析为带版面的 Markdown 文本。
// 空文本（纯图片 PDF）返回 "" 且 err==nil——调用方据此回退旧链路。
func PDFToMarkdown(raw []byte) (out string, err error) {
	// sidecar 契约：任何失败都必须能回退 trpc reader。ledongthuc Content() 无
	// recover（viewer 可容忍的畸形 content stream——不平衡 Q 使 gstack 出空、
	// 元数错误的运算符 panic("bad TL") 等——只在 NewReader 解析 xref 通过后、
	// 逐页抽取时才炸），panic 会穿透 ingest worker 而非回退。在此归一化为错误。
	defer func() {
		if r := recover(); r != nil {
			out = ""
			err = fmt.Errorf("parse pdf: panic: %v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("parse pdf: %w", err)
	}
	var b strings.Builder
	for p := 1; p <= r.NumPage(); p++ {
		page := r.Page(p)
		if page.V.IsNull() {
			continue
		}
		lines, cellW := clusterPageLines(page.Content().Text)
		text := renderPageMarkdown(lines, cellW)
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&b, "<!-- page:%d -->\n\n", p)
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), nil
}

// textCell 行内单元：文本 + 首字符起始 X（列对齐的几何依据）。
type textCell struct {
	text string
	x    float64
}

// pageLine 一行视觉文本（字符已按 X 升序，整行共基线）。
type pageLine struct {
	y     float64
	parts []pdf.Text
}

// clusterPageLines 把逐字内容点按基线 Y 聚组为行：先按 Y 降序（阅读序）排，
// 相邻字的 |ΔY| 超容差即断行；行内再按 X 升序。同时返回页面字符中位宽 cellW。
func clusterPageLines(chars []pdf.Text) ([]pageLine, float64) {
	pts := relayout(mergeVerticalRuns(chars))
	filtered := pts[:0]
	for _, c := range pts {
		// 只滤空串；空白字形作为字面空格保留——字符追踪（Tc>0）把 core-14 零宽
		// 字摊到真实 X 上时，间隙几何不足以还原词间空格（AFM 估值 overshoot 后一
		// 字），过滤会把 "Deployment Runbook" 熔成 "DeploymentRunbook"（旧
		// GetPlainText 逐字保留空格，属回归）。行尾空格由 splitLineCells 收尾
		// TrimSpace 处理；列判定只看逐字 X 几何，空格驻留不改变列缝行为。
		if c.S == "" {
			continue
		}
		filtered = append(filtered, c)
	}
	pts = filtered
	if len(pts) == 0 {
		return nil, 6.0
	}
	cellW := medianCharWidth(pts)
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].Y != pts[j].Y {
			return pts[i].Y > pts[j].Y
		}
		return pts[i].X < pts[j].X
	})
	var lines []pageLine
	tol := cellW * lineYClusterFactor
	for _, c := range pts {
		if n := len(lines); n > 0 {
			last := &lines[n-1]
			if last.y-c.Y <= tol && c.Y-last.y >= -tol {
				last.parts = append(last.parts, c)
				continue
			}
		}
		lines = append(lines, pageLine{y: c.Y, parts: []pdf.Text{c}})
	}
	for i := range lines {
		sort.Slice(lines[i].parts, func(a, b int) bool { return lines[i].parts[a].X < lines[i].parts[b].X })
	}
	return lines, cellW
}

// relayout 对零宽字（core-14 无 /Widths，ledongthuc 不内置 AFM 度量：g.Tm 无
// 推进，同一 Tj 块的字符 X 全部塌缩在块原点）按内容序重建逐字位置：同 Y 同 X
// 的连续塌缩段视为一个 showText 块，从其共同 X 起按 AFM 近似宽度依次铺开；空
// 白字形也参与铺开，被过滤后其后一字的间隙仍然真实。内嵌字体（W>0）原样保留。
func relayout(chars []pdf.Text) []pdf.Text {
	out := make([]pdf.Text, 0, len(chars))
	for i := 0; i < len(chars); {
		c := chars[i]
		if c.W > 0 {
			out = append(out, c)
			i++
			continue
		}
		x := c.X
		j := i
		for j < len(chars) && chars[j].W <= 0 && chars[j].X == c.X && chars[j].Y == c.Y {
			nc := chars[j]
			w := afmWidth(nc.Font, firstRune(nc.S)) / 1000 * nc.FontSize
			nc.X = x
			nc.W = w
			x += w
			out = append(out, nc)
			j++
		}
		i = j
	}
	return out
}

// verticalRunXTolerance 同一竖排 showText 段逐字 X 抖动容差（pt，绝对值）。
const verticalRunXTolerance = 2.0

// mergeVerticalRuns 旋转文本修复：旋转 CTM（如 `0 1 -1 0 x y cm`）下
// Trm = Tm·CTM 的 [0][0] 分量为 0，ledongthuc 据此填的 W 与 FontSize 双双塌 0，
// 但内容流仍按阅读序逐字排放（X 恒定、Y 步进）。横向重排路径把每个字当成
// 独立零宽 cell：Y 降序聚行后竖排标签被拆成单字倒序行（旋转表头/侧标/水印
// 全碎）。这里把同 X 的连续零尺寸字按流序合并为一个单元，竖排标签保持完整。
// 非旋转零宽字（core-14 无 /Widths 的横向文本）FontSize>0 不受影响。
func mergeVerticalRuns(chars []pdf.Text) []pdf.Text {
	out := make([]pdf.Text, 0, len(chars))
	for i := 0; i < len(chars); {
		c := chars[i]
		if c.W > 0 || c.FontSize > 0 {
			out = append(out, c)
			i++
			continue
		}
		var sb strings.Builder
		j := i
		for j < len(chars) && chars[j].W <= 0 && chars[j].FontSize <= 0 && abs(chars[j].X-c.X) <= verticalRunXTolerance {
			sb.WriteString(chars[j].S)
			j++
		}
		merged := c
		merged.S = sb.String()
		out = append(out, merged)
		i = j
	}
	return out
}

// afmWidth core-14 字体的近似字宽（glyph 单位 /1000）。Courier 系等宽精确
// （600）；比例字体用 Helvetica 近似分档——阈值均以页面中位宽自校准，分档误
// 差只影响词内间隙的相对判定。内嵌字体不走这里（W 来自 /Widths）。
func afmWidth(font string, r rune) float64 {
	if strings.Contains(strings.ToLower(font), "courier") {
		return 600
	}
	if r >= 0x2E80 {
		return 1000 // CJK 全宽（core 字体一般不含，兜底）
	}
	switch {
	case r == ' ':
		return 278
	case strings.ContainsRune("ijlftI.,;:'!|", r):
		return 278
	case strings.ContainsRune("mwMW", r):
		return 778
	default:
		return 556
	}
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// medianCharWidth 页面字符中位宽（pt），无样本兜底 6pt。
func medianCharWidth(chars []pdf.Text) float64 {
	var ws []float64
	for _, c := range chars {
		if c.W > 0 {
			ws = append(ws, c.W)
		}
	}
	if len(ws) == 0 {
		return 6.0
	}
	sort.Float64s(ws)
	return ws[len(ws)/2]
}

// medianSpacing 相邻行 Y 差的中位数（行距基线，pt）。
func medianSpacing(lines []pageLine) float64 {
	var gaps []float64
	for i := 1; i < len(lines); i++ {
		if g := lines[i-1].y - lines[i].y; g > 0 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return 12.0
	}
	sort.Float64s(gaps)
	return gaps[len(gaps)/2]
}

// renderPageMarkdown 行簇 → Markdown：先按几何列缝切单元，接着行组探测表格
// （第 2 列起点对齐），非表格行按段落行距输出文本。
func renderPageMarkdown(lines []pageLine, cellW float64) string {
	if len(lines) == 0 {
		return ""
	}
	rowGapMed := medianSpacing(lines)
	cells := make([][]textCell, len(lines))
	for i, ln := range lines {
		cells[i] = splitLineCells(ln, cellW)
	}

	var out []string
	for i := 0; i < len(lines); {
		if tableRunStart(cells, i) {
			end := tableRunEnd(cells, i)
			out = append(out, emitMarkdownTable(cells[i:end])...)
			i = end
			continue
		}
		if len(out) > 0 && out[len(out)-1] != "" && lines[i-1].y-lines[i].y > rowGapMed*paragraphGapFactor {
			out = append(out, "")
		}
		out = append(out, joinCellsAsText(cells[i]))
		i++
	}
	return strings.Join(out, "\n")
}

// splitLineCells 按逐字真实宽计算结束位 X；间隙超阈值切新单元。
func splitLineCells(ln pageLine, cellW float64) []textCell {
	var cells []textCell
	var cur strings.Builder
	curX := 0.0
	endX := 0.0
	for i, p := range ln.parts {
		gap := p.X - endX
		if i > 0 && gap > cellW*colGapMinFactor {
			cells = append(cells, textCell{text: strings.TrimSpace(cur.String()), x: curX})
			cur.Reset()
			curX = p.X
		} else if i > 0 && needsJoinSpace(cur.String(), p.S, gap, cellW) {
			cur.WriteByte(' ')
		}
		if i == 0 {
			curX = p.X
		}
		cur.WriteString(p.S)
		e := p.W
		if e <= 0 {
			e = cellW * widthFactor(lastRune(p.S)) // W 缺失时按宽比兜底
		}
		endX = p.X + e
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		cells = append(cells, textCell{text: s, x: curX})
	}
	return cells
}

// widthFactor 字符宽的相对比（CJK=1，拉丁/数字/空格约 0.55）——仅 W 缺失兜底。
func widthFactor(r rune) float64 {
	if r >= 0x2E80 {
		return 1.0
	}
	return 0.55
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

// needsJoinSpace 拉丁-拉丁片段间补空格（CJK 相邻无需）；近零间隙（同一视觉
// 词内切片）不补。
func needsJoinSpace(prev, next string, gap, cellW float64) bool {
	if gap < cellW*0.22 || prev == "" || next == "" {
		return false
	}
	rp, rn := []rune(prev), []rune(next)
	last, first := rp[len(rp)-1], rn[0]
	if last >= 0x2E80 || first >= 0x2E80 {
		return false
	}
	return true
}

// tableRunStart 报告第 i 行起是否存在 ≥minTableRows 行、列对齐的多列块。
// 对齐判据：块内各行列数 ≥2 且第 2 列起点 X 落容差带（第 1 列普遍同左缘，
// 对齐无判别力）。
func tableRunStart(cells [][]textCell, i int) bool {
	if i+minTableRows > len(cells) {
		return false
	}
	base, ok := secondColX(cells[i])
	if !ok {
		return false
	}
	for j := i; j < i+minTableRows; j++ {
		if len(cells[j]) < 2 {
			return false
		}
		x, ok := secondColX(cells[j])
		if !ok || abs(x-base) > colAlignTolerance {
			return false
		}
	}
	return true
}

func secondColX(cells []textCell) (float64, bool) {
	if len(cells) < 2 {
		return 0, false
	}
	return cells[1].x, true
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// tableRunEnd 吞掉连续多列行（块内列数可波动；单列行视为表格终止）。
// 续行同样受 tableRunStart 的对齐约束：第 2 列起点漂出容差即终止——否则块尾
// 紧随的键值行/脚注会被吸进表体，错位补出垃圾「行→表头」映射。
func tableRunEnd(cells [][]textCell, i int) int {
	base, ok := secondColX(cells[i])
	if !ok {
		return i
	}
	j := i
	for j < len(cells) && len(cells[j]) >= 2 {
		x, ok := secondColX(cells[j])
		if !ok || abs(x-base) > colAlignTolerance {
			break
		}
		j++
	}
	return j
}

// emitMarkdownTable 首行当表头，后续按行铺开；| 转义，列数不齐补空单元。
func emitMarkdownTable(run [][]textCell) []string {
	width := 0
	for _, row := range run {
		if len(row) > width {
			width = len(row)
		}
	}
	pad := func(row []textCell) []string {
		out := make([]string, width)
		for k := range out {
			if k < len(row) {
				out[k] = strings.ReplaceAll(strings.TrimSpace(row[k].text), "|", "\\|")
			}
		}
		return out
	}
	out := make([]string, 0, len(run)+2)
	out = append(out, "| "+strings.Join(pad(run[0]), " | ")+" |")
	seps := make([]string, width)
	for k := range seps {
		seps[k] = "---"
	}
	out = append(out, "| "+strings.Join(seps, " | ")+" |")
	for _, row := range run[1:] {
		out = append(out, "| "+strings.Join(pad(row), " | ")+" |")
	}
	out = append(out, "")
	return out
}

// joinCellsAsText 非表格行：多单元行以双空格连接为单行文本。
func joinCellsAsText(cells []textCell) string {
	parts := make([]string, 0, len(cells))
	for _, c := range cells {
		parts = append(parts, c.text)
	}
	return strings.Join(parts, "  ")
}
