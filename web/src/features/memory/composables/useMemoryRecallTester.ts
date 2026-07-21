// FD4 fix: extract recall debug + composite search data fetching from
// MemoryRecallTesterPanel.vue into composable so the .vue file only handles template.
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CompositeSearchHit, MemoryRecallHit } from '../types';
import { useMemoryApi } from './useMemoryApi';

export function useMemoryRecallTester(agentId: () => string | null, sessionId: () => string | null | undefined) {
  const { t } = useI18n();
  const { compositeSearchMemories, debugMemoryRecall } = useMemoryApi();
  const l2Hits = ref<MemoryRecallHit[]>([]);
  const l3Hits = ref<MemoryRecallHit[]>([]);
  const compositeHits = ref<CompositeSearchHit[]>([]);
  const loadingDebug = ref(false);
  const loadingComposite = ref(false);
  const error = ref('');

  watch(agentId, () => {
    l2Hits.value = [];
    l3Hits.value = [];
    compositeHits.value = [];
    error.value = '';
  });

  async function runDebug(query: string, l2Limit: number, l3Limit: number) {
    const aid = agentId();
    if (!aid || !query.trim()) return;
    loadingDebug.value = true;
    error.value = '';
    try {
      const res = await debugMemoryRecall({
        agent_id: aid,
        session_id: sessionId() || undefined,
        query: query.trim(),
        l2_limit: l2Limit,
        l3_limit: l3Limit,
      });
      l2Hits.value = res.l2_hits;
      l3Hits.value = res.l3_hits;
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.recall.debugFailed');
    } finally {
      loadingDebug.value = false;
    }
  }

  async function runComposite(query: string) {
    const aid = agentId();
    if (!aid || !query.trim()) return;
    loadingComposite.value = true;
    error.value = '';
    try {
      compositeHits.value = await compositeSearchMemories({
        agent_id: aid,
        session_id: sessionId() || undefined,
        query: query.trim(),
        limit: 10,
      });
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.recall.compositeFailed');
    } finally {
      loadingComposite.value = false;
    }
  }

  return { l2Hits, l3Hits, compositeHits, loadingDebug, loadingComposite, error, runDebug, runComposite };
}
