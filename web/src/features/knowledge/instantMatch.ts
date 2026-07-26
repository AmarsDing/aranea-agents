/**
 * instantMatch：即时区 fzf 式纯前端过滤（P3-2，<10k 文档内存索引）。
 *
 * 算法：子序列匹配（大小写不敏感）+ 连续命中/词边界加分；多词条（空格分隔）
 * 为 AND 语义——每个词必须至少命中一个字段，得分求和。无向量、无后端往返。
 */

/** fzfScore 返回 query 作为子序列在 text 中的得分；-1 = 不匹配。 */
export function fzfScore(query: string, text: string): number {
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  if (!q) return 0;
  let qi = 0;
  let score = 0;
  let prevMatch = -2;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      score += 1;
      if (ti === prevMatch + 1) score += 3; // 连续命中加分
      if (ti === 0 || /[\s/\\._-]/.test(t[ti - 1])) score += 2; // 词边界加分
      prevMatch = ti;
      qi++;
    }
  }
  return qi === q.length ? score : -1;
}

/**
 * instantFilter 多词条 AND 过滤：每个空格分隔的词必须命中至少一个字段，
 * 按总分降序返回前 limit 条。query 为空返回空数组（由调用方决定展示态）。
 */
export function instantFilter<T>(items: readonly T[], query: string, keys: (item: T) => string[], limit = 20): T[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (!terms.length) return [];
  const scored: { item: T; score: number }[] = [];
  for (const item of items) {
    const fields = keys(item);
    let total = 0;
    let allHit = true;
    for (const term of terms) {
      let best = -1;
      for (const f of fields) {
        const s = fzfScore(term, f);
        if (s > best) best = s;
      }
      if (best < 0) {
        allHit = false;
        break;
      }
      total += best;
    }
    if (allHit) scored.push({ item, score: total });
  }
  scored.sort((a, b) => b.score - a.score);
  return scored.slice(0, limit).map((s) => s.item);
}
