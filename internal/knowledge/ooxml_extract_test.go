package knowledge

// G-01 回归：.xlsx/.pptx 自研 OOXML 抽取 + .doc 明确拒绝。
// 修复前白名单放行这三种格式但 trpc reader 无实现，摄取必然 "no document reader"。

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

// buildZip 把部件清单打包为 zip 字节（内存构造 OOXML 样例）。
func buildZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// minimalXLSX 构造含共享字符串与字面数字单表工作簿。
func minimalXLSX(t *testing.T) []byte {
	t.Helper()
	return buildZip(t, map[string]string{
		"xl/workbook.xml": `<?xml version="1.0"?>
<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="月度巡检" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0"?>
<sst><si><t>机柜</t></si><si><t>温度℃</t></si><si><t>A1</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?>
<worksheet><sheetData>
  <row r="1">
    <c r="A1" t="s"><v>0</v></c>
    <c r="B1" t="s"><v>1</v></c>
    <c r="C1" t="s"><v>2</v></c>
  </row>
  <row r="2">
    <c r="A2"><v>23.5</v></c>
    <c r="B2"><v>41</v></c>
    <c r="C2" t="inlineStr"><is><t>A1</t></is></c>
  </row>
</sheetData></worksheet>`,
	})
}

// minimalPPTX 构造两页幻灯片（含 rich text run）。
func minimalPPTX(t *testing.T) []byte {
	t.Helper()
	slide := func(body string) string {
		return `<?xml version="1.0"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
       xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <p:cSld><p:spTree>` + body + `</p:spTree></p:cSld>
</p:sld>`
	}
	para := func(runs ...string) string {
		var b strings.Builder
		b.WriteString(`<a:p>`)
		for _, r := range runs {
			b.WriteString(`<a:r><a:t>` + r + `</a:t></a:r>`)
		}
		b.WriteString(`</a:p>`)
		return b.String()
	}
	return buildZip(t, map[string]string{
		"ppt/slides/slide1.xml": slide(para("变更窗口通告", "（冻结期）") + para("每周三 22:00-24:00")),
		"ppt/slides/slide2.xml": slide(para("回滚预案") + para("先回滚数据库")),
	})
}

func TestExtractXLSXText(t *testing.T) {
	ex := NewTextExtractor()
	text, err := ex.Extract(context.Background(), minimalXLSX(t), "report.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err != nil {
		t.Fatalf("xlsx extract err: %v", err)
	}
	for _, want := range []string{"## 月度巡检", "机柜", "温度℃", "23.5", "41"} {
		if !strings.Contains(text, want) {
			t.Fatalf("xlsx extract missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "| 机柜 |") {
		t.Fatalf("xlsx rows should render as pipe rows:\n%s", text)
	}
}

func TestExtractPPTXText(t *testing.T) {
	ex := NewTextExtractor()
	text, err := ex.Extract(context.Background(), minimalPPTX(t), "deck.pptx",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation")
	if err != nil {
		t.Fatalf("pptx extract err: %v", err)
	}
	for _, want := range []string{"## Slide 1", "## Slide 2", "变更窗口通告（冻结期）", "回滚预案", "先回滚数据库"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pptx extract missing %q:\n%s", want, text)
		}
	}
	// 页序：slide1 必须早于 slide2。
	if strings.Index(text, "## Slide 1") > strings.Index(text, "## Slide 2") {
		t.Fatalf("slides out of order:\n%s", text)
	}
}

func TestExtractLegacyDocRejected(t *testing.T) {
	ex := NewTextExtractor()
	_, err := ex.Extract(context.Background(), []byte("\xd0\xcf\x11\xe0 binary"), "old.doc", "application/msword")
	if err == nil {
		t.Fatal("legacy .doc must be rejected explicitly")
	}
	if !strings.Contains(err.Error(), ".docx") && !strings.Contains(err.Error(), "PDF") {
		t.Fatalf("error should guide conversion, got: %v", err)
	}
}

func TestExtractOOXMLCorrupt(t *testing.T) {
	ex := NewTextExtractor()
	if _, err := ex.Extract(context.Background(), []byte("not a zip"), "broken.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err == nil {
		t.Fatal("corrupt xlsx must error, not panic")
	}
}
