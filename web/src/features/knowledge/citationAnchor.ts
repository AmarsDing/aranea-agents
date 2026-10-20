// G-17：chat 引用锚定——notice line（chunk 开头截 120 字符的预览）在目标文档
// 正文中的锚点定位（纯函数，可测）。
// 不变量：tool.go 的 emit line = chunk 内容 trim 后的开头前缀，故锚点 = 归一化
// 后首个样本匹配处的 raw 起点（与 G-18 的 context 邻近消歧不同——这里 line 即
// chunk 首句，天然定位 chunk 起点）。
// 归一化：空白折叠为单空格并记录 raw 索引映射，内容/样本同口径折算；
// 无匹配时样本按 3/4 递减重试（收尾换行/截断差异），递减到 MIN_SAMPLE 仍失
// 败即返回 -1（长度不再收敛时强制停止，杜绝定点死循环）。

const LINE_SAMPLE = 96;
const MIN_SAMPLE = 12;

const TAIL_ELLIPSIS_RE = /(?:…|\.\.\.)\s*$/;

/** 空白折叠归一化 + 每字符 raw 索引映射（折叠掉的字符不入映射）。
 *  G-17/G-18 锚定共用的同一坐标系折算（backlinkAnchor 依赖 orig 单调递增做
 *  raw→norm 反查）。 */
export function normWithMap(raw: string): { text: string; orig: number[] } {
  const out: string[] = [];
  const orig: number[] = [];
  let pending = -1;
  for (let i = 0; i < raw.length; i++) {
    const ch = raw[i];
    if (/\s/.test(ch)) {
      if (out.length) pending = i; // 首个空白占位使折行命中可溯源
      continue;
    }
    if (pending >= 0) {
      out.push(' ');
      orig.push(pending);
      pending = -1;
    }
    out.push(ch);
    orig.push(i);
  }
  return { text: out.join(''), orig };
}

/**
 * G-17 主入口：line → 正文锚点偏移。
 * @returns scrollToOffset 可消费的 raw 偏移；定位失败 -1。
 */
export function findCitationOffset(content: string, line: string): number {
  if (!content || !line) return -1;
  const nContent = normWithMap(content);
  let sample = normWithMap(line.replace(TAIL_ELLIPSIS_RE, '')).text;
  if (!sample) return -1;
  sample = sample.slice(0, LINE_SAMPLE);
  for (;;) {
    const at = nContent.text.indexOf(sample);
    if (at >= 0) return nContent.orig[at] ?? 0;
    if (sample.length <= MIN_SAMPLE) return -1;
    // 截断样本能落在 raw 结尾/解码差异上——递减重试而非取子串首现（避免
    // 把 chunk 锚到句子中段）。长度恰为 MIN_SAMPLE 时 0.75 收敛会原地
    // 不动（定点），故显式判停。
    const next = sample.slice(0, Math.max(MIN_SAMPLE, Math.ceil(sample.length * 0.75))).trimEnd();
    if (next.length >= sample.length) return -1;
    sample = next;
  }
}
