// Word / Excel / PowerPoint 的「原样」预览（简版）：浏览器自带的 DecompressionStream 解 zip、DOMParser 读 XML，
// 不引第三方库。保留标题层级、粗斜体下划线、列表、表格；表格按工作表出表格；幻灯片按页出文字。
// 输出只含本模块拼的标签 + 转义过的文字，可直接 v-html。
// shortcut: 不还原字体、颜色、图片、合并单元格、图表；要像素级还原得接 LibreOffice 转 PDF。

export type OfficeKind = 'docx' | 'xlsx' | 'pptx';

const MAX_SHEET_ROWS = 500;
const MAX_SHEET_COLS = 60;
const MAX_SHEETS = 10;

export function officeKindOf(name: string): OfficeKind | '' {
  const ext = name.split('.').pop()?.toLowerCase() ?? '';
  return ext === 'docx' || ext === 'xlsx' || ext === 'pptx' ? ext : '';
}

export function esc(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
}

// ---------- zip ----------

async function inflateRaw(data: Uint8Array): Promise<Uint8Array> {
  const stream = new Blob([data.slice()]).stream().pipeThrough(new DecompressionStream('deflate-raw'));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

/** 读 zip 中央目录，按需解出 wanted 判定为 true 的条目（只支持不压缩 / deflate）。 */
export async function unzip(buf: ArrayBuffer, wanted: (name: string) => boolean): Promise<Map<string, string>> {
  const v = new DataView(buf);
  const bytes = new Uint8Array(buf);
  let eocd = -1;
  for (let i = bytes.length - 22; i >= Math.max(0, bytes.length - 65_557); i--) {
    if (v.getUint32(i, true) === 0x06054b50) {
      eocd = i;
      break;
    }
  }
  if (eocd < 0) throw new Error('not a zip file');
  const count = v.getUint16(eocd + 10, true);
  let p = v.getUint32(eocd + 16, true);
  const out = new Map<string, string>();
  const dec = new TextDecoder('utf-8');
  for (let n = 0; n < count && v.getUint32(p, true) === 0x02014b50; n++) {
    const method = v.getUint16(p + 10, true);
    const csize = v.getUint32(p + 20, true);
    const nameLen = v.getUint16(p + 28, true);
    const extraLen = v.getUint16(p + 30, true);
    const commentLen = v.getUint16(p + 32, true);
    const local = v.getUint32(p + 42, true);
    const name = dec.decode(bytes.subarray(p + 46, p + 46 + nameLen));
    p += 46 + nameLen + extraLen + commentLen;
    if (!wanted(name)) continue;
    const start = local + 30 + v.getUint16(local + 26, true) + v.getUint16(local + 28, true);
    const raw = bytes.subarray(start, start + csize);
    if (method === 0) out.set(name, dec.decode(raw));
    else if (method === 8) out.set(name, dec.decode(await inflateRaw(raw)));
  }
  return out;
}

// ---------- XML 小工具（按本地名找，不管命名空间前缀） ----------

function xml(s: string | undefined): Document | null {
  return s ? new DOMParser().parseFromString(s, 'application/xml') : null;
}

function kids(el: Element, local: string): Element[] {
  return Array.from(el.children).filter((c) => c.localName === local);
}

function all(el: Element | Document, local: string): Element[] {
  return Array.from(el.getElementsByTagName('*')).filter((c) => c.localName === local);
}

function attr(el: Element | undefined, local: string): string {
  if (!el) return '';
  // 有的解析器把带前缀的属性名整个当本地名（w:val），两种都认
  for (const a of Array.from(el.attributes)) if (a.localName === local || a.name.endsWith(`:${local}`)) return a.value;
  return '';
}

/** 关系 id（r:id）：sldId 上同时有 id 和 r:id，只认带前缀的那个。 */
function relId(el: Element): string {
  for (const a of Array.from(el.attributes)) if (a.name.includes(':') && a.name.endsWith(':id')) return a.value;
  return '';
}

// ---------- Word ----------

function runHtml(r: Element): string {
  const pr = kids(r, 'rPr')[0];
  let text = '';
  for (const c of Array.from(r.children)) {
    if (c.localName === 't') text += esc(c.textContent ?? '');
    else if (c.localName === 'tab') text += '&emsp;';
    else if (c.localName === 'br') text += '<br>';
  }
  if (!text) return '';
  const on = (k: string) => {
    const e = pr && kids(pr, k)[0];
    return !!e && attr(e, 'val') !== '0' && attr(e, 'val') !== 'false' && attr(e, 'val') !== 'none';
  };
  if (on('b')) text = `<b>${text}</b>`;
  if (on('i')) text = `<i>${text}</i>`;
  if (on('u')) text = `<u>${text}</u>`;
  if (on('strike')) text = `<s>${text}</s>`;
  return text;
}

function paraHtml(p: Element): string {
  const pr = kids(p, 'pPr')[0];
  const style = attr(pr && kids(pr, 'pStyle')[0], 'val').toLowerCase();
  const inner = Array.from(p.children)
    .map((c) =>
      c.localName === 'r' ? runHtml(c) : c.localName === 'hyperlink' ? kids(c, 'r').map(runHtml).join('') : '',
    )
    .join('');
  const level = /^(heading|标题)\s*([1-6])$/.exec(style)?.[2] ?? (style === 'title' ? '1' : '');
  if (level) return `<h${level}>${inner || '&nbsp;'}</h${level}>`;
  if (pr && kids(pr, 'numPr').length) return `<p class="ofc-li">• ${inner}</p>`;
  return `<p>${inner || '&nbsp;'}</p>`;
}

function tableHtml(t: Element): string {
  const rows = kids(t, 'tr').map(
    (tr) =>
      `<tr>${kids(tr, 'tc')
        .map((tc) => `<td>${kids(tc, 'p').map(paraHtml).join('')}</td>`)
        .join('')}</tr>`,
  );
  return `<table class="ofc-table">${rows.join('')}</table>`;
}

export function docxHtml(files: Map<string, string>): string {
  const doc = xml(files.get('word/document.xml'));
  const body = doc && all(doc, 'body')[0];
  if (!body) return '';
  return Array.from(body.children)
    .map((c) => (c.localName === 'p' ? paraHtml(c) : c.localName === 'tbl' ? tableHtml(c) : ''))
    .join('');
}

// ---------- Excel ----------

/** 列字母 → 序号（A=0）。 */
function colIndex(ref: string): number {
  let n = 0;
  for (const ch of ref.replace(/\d+$/, '')) n = n * 26 + (ch.charCodeAt(0) - 64);
  return n - 1;
}

function colName(i: number): string {
  let s = '';
  for (let n = i + 1; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + ((n - 1) % 26)) + s;
  return s;
}

/** sheet 文件名：xl/workbook.xml 的顺序 + 关系表。 */
function sheetList(files: Map<string, string>): Array<{ name: string; path: string }> {
  const wb = xml(files.get('xl/workbook.xml'));
  const rels = xml(files.get('xl/_rels/workbook.xml.rels'));
  if (!wb) return [];
  const target = new Map<string, string>();
  for (const r of rels ? all(rels, 'Relationship') : []) target.set(attr(r, 'Id'), attr(r, 'Target'));
  return all(wb, 'sheet').map((s, i) => {
    const t = target.get(relId(s)) ?? `worksheets/sheet${i + 1}.xml`;
    return { name: attr(s, 'name') || `Sheet${i + 1}`, path: t.startsWith('/') ? t.slice(1) : `xl/${t}` };
  });
}

export function xlsxHtml(files: Map<string, string>): string {
  const ss = xml(files.get('xl/sharedStrings.xml'));
  const shared = ss
    ? all(ss, 'si').map((si) =>
        all(si, 't')
          .map((t) => t.textContent ?? '')
          .join(''),
      )
    : [];
  const out: string[] = [];
  for (const sheet of sheetList(files).slice(0, MAX_SHEETS)) {
    const doc = xml(files.get(sheet.path));
    if (!doc) continue;
    const grid: string[][] = [];
    let width = 0;
    for (const row of all(doc, 'row').slice(0, MAX_SHEET_ROWS)) {
      const r = Number(attr(row, 'r') || grid.length + 1) - 1;
      const cells: string[] = (grid[r] = grid[r] ?? []);
      kids(row, 'c').forEach((c, k) => {
        const ci = attr(c, 'r') ? colIndex(attr(c, 'r')) : k;
        if (ci >= MAX_SHEET_COLS) return;
        const type = attr(c, 't');
        const v = kids(c, 'v')[0]?.textContent ?? '';
        cells[ci] =
          type === 's'
            ? (shared[Number(v)] ?? '')
            : type === 'inlineStr'
              ? all(c, 't')
                  .map((t) => t.textContent ?? '')
                  .join('')
              : type === 'b'
                ? v === '1'
                  ? 'TRUE'
                  : 'FALSE'
                : v;
        width = Math.max(width, ci + 1);
      });
    }
    const head = `<tr><th></th>${Array.from({ length: width }, (_, i) => `<th>${colName(i)}</th>`).join('')}</tr>`;
    const body = Array.from(grid.entries())
      .filter(([, cells]) => cells?.some((x) => x !== undefined && x !== ''))
      .map(
        ([r, cells]) =>
          `<tr><th>${r + 1}</th>${Array.from({ length: width }, (_, i) => `<td>${esc(cells[i] ?? '')}</td>`).join('')}</tr>`,
      )
      .join('');
    out.push(`<h3>${esc(sheet.name)}</h3><table class="ofc-table ofc-sheet">${head}${body}</table>`);
  }
  return out.join('');
}

// ---------- PowerPoint ----------

export function pptxHtml(files: Map<string, string>): string {
  const pres = xml(files.get('ppt/presentation.xml'));
  const rels = xml(files.get('ppt/_rels/presentation.xml.rels'));
  const target = new Map<string, string>();
  for (const r of rels ? all(rels, 'Relationship') : []) target.set(attr(r, 'Id'), attr(r, 'Target'));
  // 按演示文稿里的顺序；关系 id 对不上时（个别解析器会丢掉 r:id）退回按文件名序号
  const order = (pres ? all(pres, 'sldId') : [])
    .map((s) => target.get(relId(s)))
    .filter((t): t is string => !!t)
    .map((t) => `ppt/${t}`);
  const slides = order.length
    ? order
    : Array.from(files.keys())
        .filter((k) => /^ppt\/slides\/slide\d+\.xml$/.test(k))
        .sort((a, b) => Number(/\d+/.exec(a.slice(11))?.[0]) - Number(/\d+/.exec(b.slice(11))?.[0]));
  return slides
    .map((path, i) => {
      const doc = xml(files.get(path));
      if (!doc) return '';
      const paras = all(doc, 'sp').map((sp) => {
        const isTitle = /title/i.test(attr(all(sp, 'ph')[0], 'type'));
        const lines = all(sp, 'p')
          .map((p) =>
            all(p, 't')
              .map((t) => esc(t.textContent ?? ''))
              .join(''),
          )
          .filter(Boolean);
        if (!lines.length) return '';
        return isTitle ? `<h3>${lines.join(' ')}</h3>` : lines.map((l) => `<p>${l}</p>`).join('');
      });
      return `<section class="ofc-slide"><div class="ofc-slide__n">${i + 1}</div>${paras.join('')}</section>`;
    })
    .join('');
}

/** 取回文件 → 解压需要的部分 → 出 HTML。 */
export async function officeToHtml(kind: OfficeKind, blob: Blob): Promise<string> {
  const prefix = { docx: 'word/', xlsx: 'xl/', pptx: 'ppt/' }[kind];
  const files = await unzip(await blob.arrayBuffer(), (n) => n.startsWith(prefix) && /\.(xml|rels)$/.test(n));
  return kind === 'docx' ? docxHtml(files) : kind === 'xlsx' ? xlsxHtml(files) : pptxHtml(files);
}
