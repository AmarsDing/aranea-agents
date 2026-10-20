// Package knowledge — OOXML（xlsx/pptx）自研文本抽取。
// 背景（G-01）：trpc document/reader 仅提供 pdf/docx/csv/json/markdown/text/golang/proto，
// .doc/.xlsx/.pptx 在白名单内却必然 "no document reader"。xlsx/pptx 均为 OOXML
// （zip 容器 + XML 部件），用标准库 archive/zip + encoding/xml 流式抽取即可覆盖
// 知识检索所需的文本面（表格行列、幻灯片文字）；legacy 二进制 .doc 给明确指引。
//
// 设计边界：只做"可检索文本"抽取，不还原排版/样式/日期格式（日期序列号按原值输出）。
// 输出按 Markdown 组织（## 分节 + 管道表格行），下游 Organize/Chunk 无模态差异。
package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ooxmlMaxTextBytes 抽取产物上限（防 zip 构造器撑爆内存）：超出截断并保留已得文本。
const ooxmlMaxTextBytes = 32 << 20

// openOOXML 以 zip 容器打开 OOXML 文档并建立部件名索引。
func openOOXML(raw []byte, source string) (*zip.Reader, map[string]*zip.File, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, nil, fmt.Errorf("open OOXML container %q: %w", source, err)
	}
	parts := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		parts[f.Name] = f
	}
	return zr, parts, nil
}

// openPart 打开部件流；不存在返回 nil, nil（调用方按可缺省处理）。
func openPart(parts map[string]*zip.File, name string) (io.ReadCloser, error) {
	f, ok := parts[name]
	if !ok {
		return nil, nil
	}
	return f.Open()
}

// ── XLSX ───────────────────────────────────────────────────────────────

// sharedStrings 汇总 <si> 内全部 <t> 文本（含富文本 run 拼接），返回按序字符串表。
func readSharedStrings(parts map[string]*zip.File) ([]string, error) {
	rc, err := openPart(parts, "xl/sharedStrings.xml")
	if err != nil || rc == nil {
		return nil, err
	}
	defer rc.Close()
	dec := xml.NewDecoder(rc)
	var out []string
	var cur strings.Builder
	inSI, inT := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			break // io.EOF 或截断：尽力保留已解析部分
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				cur.Reset()
			case "t":
				if inSI {
					inT = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				out = append(out, cur.String())
				inSI = false
			case "t":
				inT = false
			}
		case xml.CharData:
			if inT {
				cur.Write([]byte(t))
			}
		}
		if cur.Len() > ooxmlMaxTextBytes {
			break
		}
	}
	return out, nil
}

// sheetOrder 从 xl/workbook.xml + 其 rels 取 (部件路径, 展示名) 按声明序的表清单。
// 解析失败时回退为 xl/worksheets/sheet*.xml 数字序（无名字段）。
func sheetOrder(parts map[string]*zip.File) ([][2]string, error) {
	type rel struct{ id, target string }
	relByID := map[string]string{}
	if rc, err := openPart(parts, "xl/_rels/workbook.xml.rels"); err == nil && rc != nil {
		dec := xml.NewDecoder(rc)
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "Relationship" {
				var id, target string
				for _, a := range se.Attr {
					switch a.Name.Local {
					case "Id":
						id = a.Value
					case "Target":
						target = a.Value
					}
				}
				if id != "" && target != "" {
					relByID[id] = target
				}
			}
		}
		rc.Close()
	}

	rc, err := openPart(parts, "xl/workbook.xml")
	if err != nil || rc == nil {
		return fallbackSheetOrder(parts), err
	}
	defer rc.Close()
	dec := xml.NewDecoder(rc)
	var out [][2]string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "sheet" {
			continue
		}
		var name, rid string
		for _, a := range se.Attr {
			switch {
			case a.Name.Local == "name":
				name = a.Value
			case a.Name.Local == "id": // r:id（命名空间化后 Local 仍为 id）
				rid = a.Value
			}
		}
		target := relByID[rid]
		if target == "" {
			continue
		}
		if !strings.HasPrefix(target, "xl/") {
			target = "xl/" + strings.TrimPrefix(target, "/")
		}
		out = append(out, [2]string{target, name})
	}
	if len(out) == 0 {
		return fallbackSheetOrder(parts), nil
	}
	return out, nil
}

// fallbackSheetOrder 无 workbook 元数据时按 sheetN.xml 数字序枚举。
func fallbackSheetOrder(parts map[string]*zip.File) [][2]string {
	type named struct {
		part string
		n    int
	}
	var list []named
	for name := range parts {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			base := strings.TrimSuffix(strings.TrimPrefix(name, "xl/worksheets/sheet"), ".xml")
			if n, err := strconv.Atoi(base); err == nil {
				list = append(list, named{name, n})
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n < list[j].n })
	out := make([][2]string, 0, len(list))
	for _, it := range list {
		out = append(out, [2]string{it.part, ""})
	}
	return out
}

// readSheetText 流式解析单表：行→单元格文本列表（空行跳过）。
func readSheetText(rc io.Reader, shared []string, emit func(cells []string)) {
	dec := xml.NewDecoder(rc)
	var cells []string
	var cur strings.Builder
	inRow, inV, inT, isInline := false, false, false, false
	cellType := ""
	flushCell := func() {
		text := cur.String()
		cur.Reset()
		if cellType == "s" {
			if idx, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && idx >= 0 && idx < len(shared) {
				text = shared[idx]
			}
		}
		cells = append(cells, strings.TrimSpace(text))
		cellType = ""
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				inRow = true
				cells = cells[:0]
			case "c":
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						cellType = a.Value
					}
				}
				cur.Reset()
			case "v":
				inV = true
			case "is":
				isInline = true
			case "t":
				if isInline {
					inT = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "row":
				if inRow {
					nonEmpty := false
					for _, c := range cells {
						if c != "" {
							nonEmpty = true
							break
						}
					}
					if nonEmpty {
						emit(cells)
					}
				}
				inRow = false
			case "c":
				flushCell()
			case "v":
				inV = false
			case "is":
				isInline = false
			case "t":
				inT = false
			}
		case xml.CharData:
			if inV || inT {
				cur.Write([]byte(t))
			}
		}
	}
}

// extractXLSXText 抽取工作簿为 Markdown 分节表格文本（## 表名 + 管道行）。
func extractXLSXText(raw []byte, source string) (string, error) {
	_, parts, err := openOOXML(raw, source)
	if err != nil {
		return "", err
	}
	shared, err := readSharedStrings(parts)
	if err != nil {
		return "", fmt.Errorf("read sharedStrings of %q: %w", source, err)
	}
	sheets, err := sheetOrder(parts)
	if err != nil {
		return "", fmt.Errorf("read workbook of %q: %w", source, err)
	}
	if len(sheets) == 0 {
		return "", fmt.Errorf("no worksheets found in %q", source)
	}

	var b strings.Builder
	b.Grow(4096)
	truncated := false
	for i, sh := range sheets {
		rc, err := openPart(parts, sh[0])
		if err != nil || rc == nil {
			continue
		}
		name := sh[1]
		if name == "" {
			name = fmt.Sprintf("Sheet %d", i+1)
		}
		b.WriteString("## ")
		b.WriteString(name)
		b.WriteString("\n")
		readSheetText(rc, shared, func(cells []string) {
			row := "| " + strings.Join(cells, " | ") + " |\n"
			if b.Len()+len(row) <= ooxmlMaxTextBytes {
				b.WriteString(row)
			} else {
				truncated = true
			}
		})
		rc.Close()
		b.WriteString("\n")
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("empty content extracted from %q", source)
	}
	if truncated {
		text += "\n\n（内容过长，已截断）"
	}
	return text, nil
}

// ── PPTX ───────────────────────────────────────────────────────────────

// extractPPTXText 抽取演示文稿为 Markdown 分节文本（## Slide N + 段落行）。
func extractPPTXText(raw []byte, source string) (string, error) {
	_, parts, err := openOOXML(raw, source)
	if err != nil {
		return "", err
	}
	type slide struct {
		part string
		n    int
	}
	var slides []slide
	for name := range parts {
		if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
			base := strings.TrimSuffix(strings.TrimPrefix(name, "ppt/slides/slide"), ".xml")
			if n, err := strconv.Atoi(base); err == nil {
				slides = append(slides, slide{name, n})
			}
		}
	}
	if len(slides) == 0 {
		return "", fmt.Errorf("no slides found in %q", source)
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })

	var b strings.Builder
	b.Grow(4096)
	truncated := false
	for _, s := range slides {
		rc, err := openPart(parts, s.part)
		if err != nil || rc == nil {
			continue
		}
		fmt.Fprintf(&b, "## Slide %d\n", s.n)
		// 按 <a:p> 段落聚合成行，段内 <a:t> 连续拼接。
		dec := xml.NewDecoder(rc)
		var line strings.Builder
		inP, inT := false, false
		writeLine := func() {
			t := strings.TrimSpace(line.String())
			line.Reset()
			if t == "" {
				return
			}
			if b.Len()+len(t)+1 <= ooxmlMaxTextBytes {
				b.WriteString(t)
				b.WriteByte('\n')
			} else {
				truncated = true
			}
		}
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "p":
					inP = true
					line.Reset()
				case "t":
					inT = true
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "p":
					if inP {
						writeLine()
					}
					inP = false
				case "t":
					inT = false
				}
			case xml.CharData:
				if inT && inP {
					line.Write([]byte(t))
				}
			}
		}
		writeLine() // 文件尾无闭合段落兜底
		rc.Close()
		b.WriteString("\n")
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("empty content extracted from %q", source)
	}
	if truncated {
		text += "\n\n（内容过长，已截断）"
	}
	return text, nil
}
