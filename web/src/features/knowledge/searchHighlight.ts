// G-16：搜索结果关键词高亮（纯函数，供 useWorkbenchCommands/SearchPanel 复用）。
// 设计：
// - 命中项 = 查询词（大小写不敏感）+ CJK 连续段 bigram（滑窗二字组），与后端 CJK 全文
//   分词同族，使「巡检 机柜」式混合查询两段都吃高亮；
// - 片段窗口以首个命中词为中心截取（Obsidian 语义），高亮区间在截取后的片段坐标系内；
// - 区间输出为 [start, end) 半开并按起点归并重叠，UI 按区间切片渲染 <mark>。
export interface HighlightRange {
  start: number;
  end: number;
}

export interface SearchSnippet {
  text: string;
  marks: HighlightRange[];
}

const CJK_RUN_RE = /[぀-ヿ㐀-䶿一-鿿豈-﫿]+/g;

/** 拆分命中项：空白分词；纯 CJK 词（≥2 字）保留整词并追加 bigram；混合词保留整词 + CJK 段 bigram。 */
export function queryTerms(query: string): string[] {
  const terms = new Set<string>();
  for (const token of query.trim().split(/\s+/).filter(Boolean)) {
    terms.add(token);
    for (const run of token.match(CJK_RUN_RE) ?? []) {
      if (run.length === 1) {
        terms.add(run);
        continue;
      }
      for (let i = 0; i + 2 <= run.length; i++) terms.add(run.slice(i, i + 2));
    }
  }
  return [...terms];
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** 在文本中收集所有命中区间（大小写不敏感），排序并归并重叠。
 *  不能用 toLowerCase+indexOf：部分 Unicode 字符小写化后长度变化（如
 *  U+0130 'İ' → 'i̇' 多一码元），lower 坐标系偏移回不到原文坐标系；
 *  /gi 正则直接在原文上匹配，index/长度天然是原文坐标。 */
export function findMarks(text: string, terms: string[]): HighlightRange[] {
  const ranges: HighlightRange[] = [];
  for (const term of terms) {
    if (!term) continue;
    const re = new RegExp(escapeRegExp(term), 'gi');
    for (;;) {
      const m = re.exec(text);
      if (!m) break;
      ranges.push({ start: m.index, end: m.index + m[0].length });
      re.lastIndex = m.index + 1; // 允许跨位重叠（bigram 们互相交叠），交给归并
    }
  }
  ranges.sort((a, b) => a.start - b.start || b.end - a.end);
  const merged: HighlightRange[] = [];
  for (const r of ranges) {
    const last = merged[merged.length - 1];
    if (last && r.start <= last.end) {
      last.end = Math.max(last.end, r.end);
    } else {
      merged.push({ ...r });
    }
  }
  return merged;
}

/** 命中文本窗口 + 片段坐标系内的高亮区间。无命中时退化为前 160 字。 */
export function buildSearchSnippet(content: string, query: string, window = 160, ctxPad = 48): SearchSnippet {
  const flat = content.replace(/\s+/g, ' ').trim();
  const terms = queryTerms(query);
  const allMarks = findMarks(flat, terms);
  const first = allMarks[0];
  let start = 0;
  if (first) start = Math.max(0, first.start - ctxPad);
  const prefix = start > 0 ? '…' : '';
  const tail = start + window < flat.length ? '…' : '';
  const text = `${prefix}${flat.slice(start, start + window)}${tail}`;
  // 窗口坐标系换算：裁剪到 [start, start+window)，平移前缀长度。
  const shift = prefix.length - start;
  const marks = allMarks
    .map((m) => ({ start: m.start + shift, end: m.end + shift }))
    .filter((m) => m.end > prefix.length && m.start < text.length - tail.length)
    .map((m) => ({ start: Math.max(prefix.length, m.start), end: Math.min(text.length - tail.length, m.end) }))
    .filter((m) => m.end > m.start);
  return { text, marks };
}
