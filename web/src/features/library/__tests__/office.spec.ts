import { describe, expect, it } from 'vitest';
import { docxHtml, esc, officeKindOf, pptxHtml, unzip, xlsxHtml } from '../office';

/** 拼一个不压缩（stored）的 zip，测试用。 */
function storedZip(entries: Record<string, string>): ArrayBuffer {
  const enc = new TextEncoder();
  const parts: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(entries)) {
    const n = enc.encode(name);
    const data = enc.encode(text);
    const local = new DataView(new ArrayBuffer(30));
    local.setUint32(0, 0x04034b50, true);
    local.setUint32(18, data.length, true);
    local.setUint32(22, data.length, true);
    local.setUint16(26, n.length, true);
    parts.push(new Uint8Array(local.buffer), n, data);
    const c = new DataView(new ArrayBuffer(46));
    c.setUint32(0, 0x02014b50, true);
    c.setUint32(20, data.length, true);
    c.setUint32(24, data.length, true);
    c.setUint16(28, n.length, true);
    c.setUint32(42, offset, true);
    central.push(new Uint8Array(c.buffer), n);
    offset += 30 + n.length + data.length;
  }
  const cdSize = central.reduce((s, x) => s + x.length, 0);
  const eocd = new DataView(new ArrayBuffer(22));
  eocd.setUint32(0, 0x06054b50, true);
  eocd.setUint16(10, Object.keys(entries).length, true);
  eocd.setUint32(12, cdSize, true);
  eocd.setUint32(16, offset, true);
  const all = [...parts, ...central, new Uint8Array(eocd.buffer)];
  const out = new Uint8Array(all.reduce((s, x) => s + x.length, 0));
  let p = 0;
  for (const x of all) {
    out.set(x, p);
    p += x.length;
  }
  return out.buffer;
}

const W = 'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"';

describe('Office 预览', () => {
  it('认扩展名、转义', () => {
    expect(officeKindOf('报价.XLSX')).toBe('xlsx');
    expect(officeKindOf('a.doc')).toBe('');
    expect(esc('<b>&"')).toBe('&lt;b&gt;&amp;&quot;');
  });

  it('Word：标题、粗体、列表、表格，文字转义', async () => {
    const files = await unzip(
      storedZip({
        'word/document.xml': `<w:document ${W}><w:body>
          <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>合同</w:t></w:r></w:p>
          <w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲方</w:t></w:r><w:r><w:t>&lt;script&gt;</w:t></w:r></w:p>
          <w:p><w:pPr><w:numPr/></w:pPr><w:r><w:t>第一条</w:t></w:r></w:p>
          <w:tbl><w:tr><w:tc><w:p><w:r><w:t>单价</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
        </w:body></w:document>`,
        'other.txt': 'skip',
      }),
      (n) => n.startsWith('word/'),
    );
    expect(files.has('other.txt')).toBe(false);
    const html = docxHtml(files);
    expect(html).toContain('<h1>合同</h1>');
    expect(html).toContain('<b>甲方</b>&lt;script&gt;');
    expect(html).toContain('ofc-li');
    expect(html).toContain('<td><p>单价</p></td>');
  });

  it('Excel：共享字符串、按单元格位置排列、工作表名', () => {
    const files = new Map([
      [
        'xl/workbook.xml',
        '<workbook><sheets><sheet name="报价" sheetId="1" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></sheets></workbook>',
      ],
      [
        'xl/_rels/workbook.xml.rels',
        '<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>',
      ],
      ['xl/sharedStrings.xml', '<sst><si><t>品名</t></si><si><t>数量</t></si></sst>'],
      [
        'xl/worksheets/sheet1.xml',
        '<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c></row><row r="2"><c r="C2"><v>12</v></c></row></sheetData></worksheet>',
      ],
    ]);
    const html = xlsxHtml(files);
    expect(html).toContain('<h3>报价</h3>');
    expect(html).toContain('<th>C</th>');
    expect(html).toContain('<td>品名</td><td></td><td>数量</td>');
    expect(html).toContain('<th>2</th><td></td><td></td><td>12</td>');
  });

  it('PowerPoint：按页、标题', () => {
    const files = new Map([
      [
        'ppt/presentation.xml',
        '<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst></p:presentation>',
      ],
      [
        'ppt/_rels/presentation.xml.rels',
        '<Relationships><Relationship Id="rId2" Target="slides/slide1.xml"/></Relationships>',
      ],
      [
        'ppt/slides/slide1.xml',
        '<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>季度汇报</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:txBody><a:p><a:r><a:t>营收 +12%</a:t></a:r></a:p></p:txBody></p:sp></p:sld>',
      ],
    ]);
    const html = pptxHtml(files);
    expect(html).toContain('<h3>季度汇报</h3>');
    expect(html).toContain('<p>营收 +12%</p>');
    expect(html).toContain('ofc-slide__n">1<');
  });
});
