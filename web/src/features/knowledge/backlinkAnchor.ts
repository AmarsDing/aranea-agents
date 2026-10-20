// G-18：反链「点回引用现场」的定位算法（纯函数，可测）。
// 输入：来源文档全文 + 反链 context（引用±50 字符片段）+ raw_target（原始链接目标文本）。
// 输出：正文中锚点偏移（scrollToOffset 消费）；定位失败返回 -1。
// 策略：
//  1. 收集 [[rawTarget]] 候选锚点（容许 #heading / |alias 后缀直到 ]]；不误配 [[targetX]]；
//     rawTarget 内部空白按 \s+ 宽容匹配——Wiki 标题可含空白，编辑器折叠形态不一）；
//  2. context 非空且在正文中可定位时：取与 context 归一化位置最近的候选（真锚点位于 context 内部）；
//     候选 raw 位置经 normWithMap.orig 反查到归一化坐标——同一数组按构造对齐，不做"出现序"假设；
//  3. 否则退化为首个候选；无候选时退化为 rawTarget 裸文本首现；兜底 -1（调用方仅打开文档坐头）。

import { normWithMap } from './citationAnchor';

const ANCHOR_SAMPLE = 24;

function norm(s: string): string {
  return s.replace(/\s+/g, ' ');
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** 收集文本里所有 [[rawTarget...]] 的出现起点（#、|、] 或空白后随才算成链）。 */
function findWikiCandidates(content: string, rawTarget: string): number[] {
  // 逐字符转义后把 rawTarget 内的空白段替换为 \s+ ——候选直接在 raw 坐标系收集，
  // 与正文折叠形态（双空格/制表符/换行）解耦。
  const body = escapeRegExp(rawTarget).replace(/\s+/g, '\\s+');
  const re = new RegExp(`\\[\\[${body}(?=[#|\\]\\s])`, 'g');
  const out: number[] = [];
  for (let m = re.exec(content); m; m = re.exec(content)) out.push(m.index);
  return out;
}

/** 归一化文本中所有子串出现位置。 */
function occurrences(haystack: string, needle: string): number[] {
  const out: number[] = [];
  if (!needle) return out;
  let idx = 0;
  for (;;) {
    const at = haystack.indexOf(needle, idx);
    if (at < 0) break;
    out.push(at);
    idx = at + 1;
  }
  return out;
}

/** raw 偏移 → 归一化坐标（orig 单调递增，二分；'[' 等非空白字符必精确命中）。 */
function normIndexOf(orig: number[], raw: number): number {
  let lo = 0;
  let hi = orig.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    const v = orig[mid];
    if (v === raw) return mid;
    if (v < raw) lo = mid + 1;
    else hi = mid - 1;
  }
  return -1;
}

/** G-18 主入口：按 context 邻近度定锚。 */
export function findBacklinkOffset(content: string, context: string, rawTarget: string): number {
  if (!content || !rawTarget) return -1;
  const candidates = findWikiCandidates(content, rawTarget);
  if (candidates.length === 1) return candidates[0];

  const ctx = norm(context).trim();
  if (ctx && candidates.length) {
    // 归一化坐标系下：context 出现位置集合 → 取候选到就近 context 起点的距离最小者。
    const nContent = normWithMap(content);
    const nCands = candidates.map((r) => normIndexOf(nContent.orig, r));
    let head = ctx;
    for (let guard = 0; guard < 2; guard++) {
      if (guard === 1) head = ctx.slice(0, ANCHOR_SAMPLE);
      const spots = occurrences(nContent.text, head);
      if (!spots.length) continue;
      let best = -1;
      let bestDist = Number.MAX_SAFE_INTEGER;
      for (let i = 0; i < nCands.length; i++) {
        if (nCands[i] < 0) continue;
        for (const s of spots) {
          const d = Math.abs(nCands[i] - s);
          if (d < bestDist) {
            bestDist = d;
            best = i;
          }
        }
      }
      if (best >= 0) return candidates[best];
    }
  }

  if (candidates.length) return candidates[0];
  return content.indexOf(rawTarget);
}
