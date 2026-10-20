/*
	G-05 起步：PDF 版面感知抽取回归——行重构/几何列缝表格/页码锚点/段落分居。

单元层用合成行（逐字真实宽 W）固化几何规则；集成层用 fpdf 生成真 PDF
（Courier 等宽字体令字符宽数学确定），端到端验证 Markdown 产出。
*/
package parser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"
	"github.com/ledongthuc/pdf"
)

// line 合成一行：xw 为 (文本, 起始X) 对；W 按 CJK=6pt/拉丁=3pt 真实宽填充。
func line(y float64, xw ...any) pageLine {
	parts := make([]pdf.Text, 0, len(xw)/2)
	for i := 0; i+1 < len(xw); i += 2 {
		s := xw[i].(string)
		w := 0.0
		for _, r := range s {
			if r >= 0x2E80 {
				w += 6
			} else {
				w += 3
			}
		}
		parts = append(parts, pdf.Text{S: s, X: xw[i+1].(float64), Y: y, W: w})
	}
	return pageLine{y: y, parts: parts}
}

// --- splitLineCells ---

func TestSplitLineCells_ColumnGap(t *testing.T) {
	ln := line(100, "Name", 20.0, "SLA", 160.0)
	cells := splitLineCells(ln, 6.0)
	if len(cells) != 2 || cells[0].text != "Name" || cells[1].text != "SLA" {
		t.Fatalf("cells = %#v", cells)
	}
}

func TestSplitLineCells_CJKJoinNoSpace(t *testing.T) {
	// 同一视觉行被内容流切成两个 CJK 片段时窄间隙直连，不插空格。
	// "部署手" 宽 18pt 止 58；"册"@60 → 间隙 2pt < 1.45*6，CJK 邻接不补空格。
	ln := line(100, "部署手", 40.0, "册", 60.0)
	cells := splitLineCells(ln, 6.0)
	if len(cells) != 1 || cells[0].text != "部署手册" {
		t.Fatalf("cells = %#v", cells)
	}
}

func TestSplitLineCells_LatinJoinSpace(t *testing.T) {
	// "Hello"+"World" 小缝拼接需补空格，否则会读成 HelloWorld。
	// "Hello" 宽 15pt 止 55；"World"@60 → 间隙 5pt ∈ [0.22, 1.45]*5 → 补空格。
	ln := line(100, "Hello", 40.0, "World", 60.0)
	cells := splitLineCells(ln, 5.0)
	if len(cells) != 1 || cells[0].text != "Hello World" {
		t.Fatalf("cells = %#v", cells)
	}
}

// --- 表格块判定 ---

func TestTableRunStart_RequiresThreeAlignedRows(t *testing.T) {
	block := [][]textCell{
		{{"Name", 20}, {"SLA", 160}},
		{{"Ops", 20}, {"99.9", 161}},  // 2 列对齐
		{{"SRE", 20}, {"99.99", 159}}, // 3 行成块
	}
	for _, tt := range []struct {
		name string
		rows [][]textCell
		i    int
		want bool
	}{
		{"three aligned rows form a table", block, 0, true},
		{"two rows only are not a table", block[:2], 0, false},
		{"table may start mid-page after prose", append([][]textCell{{{"只有一列", 40}}}, block...), 1, true},
		{"drifting second column breaks alignment", [][]textCell{
			{{"A", 20}, {"x", 160}},
			{{"B", 20}, {"y", 300}}, // 第 2 列起点漂移 → 非同一表格
			{{"C", 20}, {"z", 160}},
		}, 0, false},
		{"second column missing breaks table", [][]textCell{
			{{"A", 20}, {"x", 160}},
			{{"只有一列", 20}},
			{{"C", 20}, {"z", 160}},
		}, 0, false},
	} {
		if got := tableRunStart(tt.rows, tt.i); got != tt.want {
			t.Errorf("%s: tableRunStart(%d) = %v, want %v", tt.name, tt.i, got, tt.want)
		}
	}
}

// --- emitMarkdownTable ---

func TestEmitMarkdownTable_EscapesPipesAndPads(t *testing.T) {
	run := [][]textCell{
		{{"名称", 20}, {"SLA|承诺", 160}},
		{{"运维", 20}, {"99.9", 160}, {"备注", 260}},
	}
	out := emitMarkdownTable(run)
	if len(out) != 4 { // header + sep + 1 row + trailing blank
		t.Fatalf("out = %#v", out)
	}
	if !strings.Contains(out[0], "SLA\\|承诺") {
		t.Errorf("header pipe not escaped: %q", out[0])
	}
	if !strings.Contains(out[1], "| --- | --- | --- |") {
		t.Errorf("separator not padded to 3 cols: %q", out[1])
	}
}

// --- renderPageMarkdown ---

func TestRenderPageMarkdown_TableAndProse(t *testing.T) {
	lines := []pageLine{
		line(200, "部署手册", 40.0),
		line(180, "Name", 40.0, "SLA", 200.0),
		line(168, "Ops", 40.0, "99.9", 200.0),
		line(156, "SRE", 40.0, "99.99", 200.0),
		line(120, "以上为值班指标。", 40.0),
	}
	md := renderPageMarkdown(lines, 6.0)
	for _, want := range []string{"部署手册", "| Name | SLA |", "| Ops | 99.9 |", "以上为值班指标。"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| 部署手册") {
		t.Errorf("prose line wrongly tabulated:\n%s", md)
	}
}

func TestRenderPageMarkdown_IdenticalTextNotTable(t *testing.T) {
	// 自然段落换行恰好同型，两行不得砌成表（count<3）。
	lines := []pageLine{
		line(200, "第一章", 40.0, "总则", 200.0),
		line(188, "第二章", 40.0, "范围", 200.0),
	}
	md := renderPageMarkdown(lines, 6.0)
	if strings.Contains(md, "| --- |") {
		t.Errorf("two-row prose wrongly tabulated:\n%s", md)
	}
}

// --- clusterPageLines ---

func TestClusterPageLines_GroupsByBaseline(t *testing.T) {
	chars := []pdf.Text{
		{S: "A", X: 40, Y: 100, W: 3},
		{S: "B", X: 43, Y: 100.5, W: 3}, // 同行微偏（上下标）
		{S: "C", X: 40, Y: 86, W: 3},    // 换行
		{S: " ", X: 43, Y: 86, W: 3},    // 空白字形保留为零宽位置符（Tc 融合回归防线）
		{S: "D", X: 46, Y: 86, W: 3},
	}
	lines, _ := clusterPageLines(chars)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", lines)
	}
	if got := lines[0].parts; len(got) != 2 || got[0].S != "A" || got[1].S != "B" {
		t.Errorf("line0 parts = %#v", got)
	}
	if got := lines[1].parts; len(got) != 3 || got[0].S != "C" || got[1].S != " " || got[2].S != "D" {
		t.Errorf("line1 parts = %#v", got)
	}
	// 单元拼接层：字面空格原样进入 cell（不再依赖间隙推断补空格）。
	cells := splitLineCells(lines[1], 6.0)
	if len(cells) != 1 || cells[0].text != "C D" {
		t.Errorf("cells = %#v, want one cell \"C D\"", cells)
	}
}

// --- 对抗评审再查（G-05 二轮）：tableRunEnd 续行也受对齐约束 ---

func TestTableRunEnd_StopsOnColumnDrift(t *testing.T) {
	cells := [][]textCell{
		{{"Name", 20}, {"SLA", 160}},
		{{"Ops", 20}, {"99.9", 161}},
		{{"SRE", 20}, {"99.99", 159}},
		{{"注：以上", 20}, {"随运维调整", 60}}, // 第 2 列漂移 → 不得吸入表体
		{{"GTM", 20}, {"99.5", 60}},   // 同样漂移
	}
	if got := tableRunEnd(cells, 0); got != 3 {
		t.Errorf("tableRunEnd = %d, want 3（漂移行之后的键值段不进表）", got)
	}
}

// --- 集成：手工构造最小合法 PDF（xref 偏移自计算），覆盖 fpdf 表达不了的流 ---

// buildRawPDF 按固定模板拼接 5 对象 PDF；content 为第 4 号对象内容流原文。
// /BaseFont /Helvetica 不带 /Widths → core-14 零宽路径（W=0）。
func buildRawPDF(t *testing.T, content string) []byte {
	t.Helper()
	bodies := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(bodies))
	for i, body := range bodies {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(bodies)+1)
	b.WriteString("0000000000 65535 f\r\n")
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n\r\n", off) // 恰好 20 字节，符合 xref 条目定长
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(bodies)+1, xref)
	return b.Bytes()
}

// TestPDFToMarkdown_PanicBecomesError：viewer 可容忍但 ledongthuc Content()
// 会 panic 的畸形流（末尾不平衡 Q 使 gstack 出空）必须归一化为 error，
// 让 document_extract.go 的 trpc 回退分支可触发（sidecar 契约）。
func TestPDFToMarkdown_PanicBecomesError(t *testing.T) {
	raw := buildRawPDF(t, "BT /F1 12 Tf 72 720 Td (Hi) Tj ET\nQ\n")
	out, err := PDFToMarkdown(raw)
	if err == nil {
		t.Fatalf("expected error for unbalanced-Q stream, got out=%q", out)
	}
}

// TestPDFToMarkdown_RotatedTextStaysContiguous：旋转 CTM 下逐字 W/FontSize 塌 0，
// 竖排标签必须按流序合并为连续文本（修复前被拆成单字倒序行）。
func TestPDFToMarkdown_RotatedTextStaysContiguous(t *testing.T) {
	content := "BT /F1 12 Tf 72 720 Td (Body) Tj ET\n" +
		"q 0 1 -1 0 300 100 cm BT /F1 12 Tf 0 0 Td (ROT) Tj ET Q\n"
	md, err := PDFToMarkdown(buildRawPDF(t, content))
	if err != nil {
		t.Fatalf("PDFToMarkdown: %v", err)
	}
	if !strings.Contains(md, "ROT") {
		t.Errorf("rotated label shredded (want contiguous \"ROT\"):\n%s", md)
	}
	if !strings.Contains(md, "Body") {
		t.Errorf("body missing:\n%s", md)
	}
}

// TestPDFToMarkdown_CharTrackingKeepsWordSpace：core-14（无 /Widths）+ Tc>0
// 字符追踪：零宽字摊到真实 X，词间空格必须保留（修复前熔成 DeploymentRunbook）。
func TestPDFToMarkdown_CharTrackingKeepsWordSpace(t *testing.T) {
	content := "BT /F1 12 Tf 1 Tc 72 720 Td (Deployment Runbook) Tj ET\n"
	md, err := PDFToMarkdown(buildRawPDF(t, content))
	if err != nil {
		t.Fatalf("PDFToMarkdown: %v", err)
	}
	if !strings.Contains(md, "Deployment Runbook") {
		t.Errorf("word space fused under Tc>0:\n%s", md)
	}
	if strings.Contains(md, "DeploymentRunbook") {
		t.Errorf("fusion regression present:\n%s", md)
	}
}

// --- 集成：fpdf 生成真 PDF 端到端 ---

func buildTablePDF(t *testing.T) []byte {
	t.Helper()
	doc := fpdf.New("P", "mm", "A4", "")
	doc.AddPage()
	doc.SetFont("Courier", "", 10)
	doc.SetXY(15, 20)
	doc.Cell(0, 6, "Deployment Runbook")
	grid := [][]string{
		{"Name", "SLA", "Phone"},
		{"Ops", "99.9", "10086"},
		{"SRE", "99.99", "10000"},
	}
	for i, row := range grid {
		y := 40.0 + float64(i)*7
		doc.SetXY(15, y)
		doc.Cell(30, 6, row[0])
		doc.SetXY(60, y)
		doc.Cell(30, 6, row[1])
		doc.SetXY(100, y)
		doc.Cell(30, 6, row[2])
	}
	doc.SetXY(15, 80)
	doc.Cell(0, 6, "Deployment concludes above.")
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatalf("fpdf output: %v", err)
	}
	return buf.Bytes()
}

func TestPDFToMarkdown_Integration(t *testing.T) {
	md, err := PDFToMarkdown(buildTablePDF(t))
	if err != nil {
		t.Fatalf("PDFToMarkdown: %v", err)
	}
	if !strings.HasPrefix(md, "<!-- page:1 -->") {
		t.Errorf("missing page anchor:\n%s", md)
	}
	for _, want := range []string{"Deployment Runbook", "| Name | SLA | Phone |", "| Ops | 99.9 | 10086 |", "Deployment concludes above."} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestPDFToMarkdown_RejectsGarbage(t *testing.T) {
	if _, err := PDFToMarkdown([]byte("not a pdf")); err == nil {
		t.Fatal("expected parse error for garbage input")
	}
}
