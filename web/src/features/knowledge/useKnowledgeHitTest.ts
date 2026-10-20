// G-19 hit-test：搜不到/搜错的自助排查——同一 query 并行跑多策略（dense/sparse/rrf），
// 对比分路分解与重排前后序。调试调用走后端 DebugKnowledgeSearch（不落 access_log）。
import { ref } from 'vue';
import type { KnowledgeChunk, KnowledgeHitTestResult } from './types';
import { debugKnowledgeSearch } from './api';

export type HitTestStrategy = 'dense' | 'sparse' | 'rrf';

export type HitTestRun = {
  strategy: HitTestStrategy;
  loading: boolean;
  error: string;
  result: KnowledgeHitTestResult | null;
  elapsed_ms: number;
};

export type HitTestQuery = {
  collection_id: string;
  query: string;
  top_k: number;
  path_prefix?: string;
};

/** 可注入的调试检索函数（测试替换为假实现）。 */
export type HitTestFetcher = (q: {
  collection_id: string;
  query: string;
  top_k?: number;
  use_rerank?: boolean;
  hybrid_search?: string;
  path_prefix?: string;
}) => Promise<KnowledgeHitTestResult>;

export type KnowledgeHitTestDeps = {
  fetcher?: HitTestFetcher;
  friendlyError?: (err: unknown) => string;
};

export const HIT_TEST_STRATEGIES: readonly HitTestStrategy[] = ['dense', 'sparse', 'rrf'];

export function useKnowledgeHitTest(deps: KnowledgeHitTestDeps = {}) {
  const fetcher = deps.fetcher ?? debugKnowledgeSearch;
  const friendlyError = deps.friendlyError ?? ((e: unknown) => (e instanceof Error ? e.message : String(e)));

  const query = ref('');
  const queryPrefix = ref('');
  const topK = ref(10);
  const useRerank = ref(true);
  const runs = ref<HitTestRun[]>([]);
  const running = ref(false);

  /** 并行执行全部策略，各自独立结算（单策略失败不拖垮其他列）。 */
  async function run(collectionId: string) {
    const text = query.value.trim();
    if (!collectionId || !text || running.value) return;
    running.value = true;
    runs.value = HIT_TEST_STRATEGIES.map((strategy) => ({
      strategy,
      loading: true,
      error: '',
      result: null,
      elapsed_ms: 0,
    }));
    await Promise.all(
      runs.value.map(async (row) => {
        const start = performance.now();
        try {
          row.result = await fetcher({
            collection_id: collectionId,
            query: text,
            top_k: topK.value > 0 ? topK.value : 10,
            use_rerank: useRerank.value,
            hybrid_search: row.strategy,
            path_prefix: queryPrefix.value.trim() || undefined,
          });
        } catch (e) {
          row.error = friendlyError(e);
        } finally {
          row.elapsed_ms = Math.round(performance.now() - start);
          row.loading = false;
        }
      }),
    );
    running.value = false;
  }

  /** 重排前后序对比：rrf/融合分给出的前序排名（无 rrf 时用未重排侧分）。 */
  function preRerankRank(row: HitTestRun, chunk: KnowledgeChunk): number | null {
    const result = row.result;
    if (!result || !result.reranked) return null;
    const chunks = result.chunks;
    if (!chunks) return null;
    const basis = (c: KnowledgeChunk) =>
      (c.rrf_score || 0) > 0 ? (c.rrf_score ?? 0) : Math.max(c.dense_score ?? 0, c.sparse_score ?? 0);
    const sorted = [...chunks].sort((a, b) => basis(b) - basis(a));
    const at = sorted.findIndex((c) => c.id === chunk.id);
    return at >= 0 ? at + 1 : null;
  }

  function clear() {
    runs.value = [];
    query.value = '';
    queryPrefix.value = '';
  }

  return { query, queryPrefix, topK, useRerank, runs, running, run, preRerankRank, clear };
}
