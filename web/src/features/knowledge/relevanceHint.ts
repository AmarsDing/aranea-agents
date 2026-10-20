/**
 * U1-6：搜索相关度分级（纯函数，供 SearchPanel 渲染相关度提示与空结果引导）。
 *
 * 分级：
 *  - empty：0 命中 → 空结果引导块（扩大范围 / 跳 hit-test；语义层特殊提示由组件按 vault 状态追加，
 *    纯函数不感知 vault 配置）；
 *  - weak：有命中但最高有效分低于 WEAK_THRESHOLD → amber 轻提示，不阻断操作；
 *  - good：达标 → 不打扰。
 *
 * 有效分：单条结果优先取有限 score，缺失/非有限（undefined/NaN/±Infinity）时回退 rrf_score，
 * 皆无记 0 —— 无法确认相关度时保守降级为提示，而不是断言。
 */

/** weak 阈值：最高有效分严格小于该值判 weak（等于即达标），导出便于调参与单测。 */
export const WEAK_THRESHOLD = 0.3;

export type RelevanceLevel = 'good' | 'weak' | 'empty';

/** 语义化提示标识（非终稿文案），组件经 TIP 键映射到 i18n 文案。 */
export type RelevanceTip =
  | 'refine-keywords'
  | 'try-rerank'
  | 'narrow-scope'
  | 'enable-semantic'
  | 'expand-scope'
  | 'open-hit-test';

export interface RelevanceResultLike {
  score?: number;
  rrf_score?: number;
}

export interface RelevanceClassification {
  level: RelevanceLevel;
  /** 所有结果有效分的最大值；无结果或全部无有效分记 0。 */
  topScore: number;
  tips: RelevanceTip[];
}

const WEAK_TIPS: readonly RelevanceTip[] = ['refine-keywords', 'try-rerank', 'narrow-scope'];
const EMPTY_TIPS: readonly RelevanceTip[] = ['expand-scope', 'open-hit-test'];

/** 单条结果有效分：score 优先，回退 rrf_score，皆无记 0。 */
function effectiveScore(r: RelevanceResultLike): number {
  if (typeof r.score === 'number' && Number.isFinite(r.score)) return r.score;
  if (typeof r.rrf_score === 'number' && Number.isFinite(r.rrf_score)) return r.rrf_score;
  return 0;
}

export function classifyResults(results: readonly RelevanceResultLike[]): RelevanceClassification {
  if (!results.length) {
    return { level: 'empty', topScore: 0, tips: [...EMPTY_TIPS] };
  }
  let topScore = 0;
  for (const r of results) {
    const s = effectiveScore(r);
    if (s > topScore) topScore = s;
  }
  if (topScore < WEAK_THRESHOLD) {
    return { level: 'weak', topScore, tips: [...WEAK_TIPS] };
  }
  return { level: 'good', topScore, tips: [] };
}
