import { computed, ref, watch, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { FactReviewAction, MemoryFact } from './types';
import { buildMemoryFactTableColumns } from './memoryTableUi';
import { useMemoryStore } from '../../stores/memory';

export function useMemoryCenterFacts(opts: {
  selectedAgentId: Ref<string | null>;
  pendingFactId: Ref<string | null>;
  onAfterFactsLoad?: () => Promise<void> | void;
}) {
  const { notify } = useQuasar();
  const { t } = useI18n();
  const memoryStore = useMemoryStore();
  const { facts } = storeToRefs(memoryStore);

  const selectedFact = ref<MemoryFact | null>(null);
  const factKeyword = ref('');
  const factScope = ref<string | null>(null);
  const factStatus = ref<string | null>('active');
  const loadingFacts = ref(false);
  const factsEndpointReady = ref(true);
  const factsTotal = ref(0);
  const factsActiveCount = ref(0);
  const factsArchivedCount = ref(0);
  const factsFilteredCount = ref(0);
  const factPage = ref(1);
  const factPageSize = ref(10);
  const factPageMax = computed(() => Math.max(1, Math.ceil(factsFilteredCount.value / factPageSize.value)));
  let factsLoadSeq = 0;
  const factDrawer = ref(false);
  const factEditOpen = ref(false);
  const factEditMode = ref<'refine' | 'create'>('refine');
  const factReviewActing = ref(false);

  const scopeOptions = computed(() =>
    ['user', 'agent', 'team', 'workspace', 'global'].map((value) => ({
      label: t(`memory.knowledge.scope.${value}`),
      value,
    })),
  );
  const factStatusOptions = computed(() =>
    ['active', 'archived', 'disputed', 'deprecated', 'deleted'].map((value) => ({
      label: t(`memory.knowledge.status.${value}`),
      value,
    })),
  );
  const factColumns = computed(() => buildMemoryFactTableColumns(formatDate, t));

  watch([factPage, factPageSize], (newVals, oldVals) => {
    const sizeChanged = newVals[1] !== oldVals[1];
    if (sizeChanged && factPage.value !== 1) {
      factPage.value = 1;
      return;
    }
    void loadFacts();
  });

  async function loadFacts() {
    const seq = ++factsLoadSeq;
    loadingFacts.value = true;
    try {
      const result = await memoryStore.loadFacts({
        keyword: factKeyword.value || undefined,
        scope_type: factScope.value || undefined,
        status: factStatus.value || undefined,
        agent_id: opts.selectedAgentId.value || undefined,
        limit: factPageSize.value,
        offset: (factPage.value - 1) * factPageSize.value,
      });
      if (seq !== factsLoadSeq) return;
      factsTotal.value = result.total;
      factsFilteredCount.value = result.filtered_count ?? result.total;
      factsActiveCount.value = result.active_count ?? 0;
      factsArchivedCount.value = result.archived_count ?? 0;
      factsEndpointReady.value = true;
      if (factPage.value > factPageMax.value) {
        factPage.value = factPageMax.value;
        return;
      }
      await opts.onAfterFactsLoad?.();
      if (opts.pendingFactId.value) {
        const found = facts.value.find((row) => row.id === opts.pendingFactId.value);
        if (found) {
          openFact(found);
          opts.pendingFactId.value = null;
        }
      }
    } catch {
      if (seq !== factsLoadSeq) return;
      memoryStore.clearFacts();
      factsTotal.value = 0;
      factsFilteredCount.value = 0;
      factsActiveCount.value = 0;
      factsArchivedCount.value = 0;
      factsEndpointReady.value = false;
    } finally {
      if (seq === factsLoadSeq) loadingFacts.value = false;
    }
  }

  async function reloadFactsFromFirstPage() {
    if (factPage.value !== 1) {
      factPage.value = 1;
      return;
    }
    await loadFacts();
  }

  function resetFactFilters() {
    factKeyword.value = '';
    factScope.value = null;
    factStatus.value = 'active';
    void reloadFactsFromFirstPage();
  }

  function openFact(row: MemoryFact) {
    selectedFact.value = row;
    factDrawer.value = true;
  }

  async function reviewSelectedFact(action: FactReviewAction) {
    const fact = selectedFact.value;
    if (!fact || factReviewActing.value) return;
    factReviewActing.value = true;
    try {
      const updated = await memoryStore.reviewFact({ fact_id: fact.id, action });
      selectedFact.value = updated;
      notify({ type: 'positive', message: t(`memory.factDrawer.reviewDone.${action}`) });
      if (action === 'archive' || action === 'deprecate') {
        await loadFacts();
        if (factStatus.value === 'active') {
          factDrawer.value = false;
        }
      }
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.factDrawer.reviewFailed') });
    } finally {
      factReviewActing.value = false;
    }
  }

  function openRefineFact() {
    if (!selectedFact.value) return;
    factEditMode.value = 'refine';
    factEditOpen.value = true;
  }

  function openCreateFact() {
    factEditMode.value = 'create';
    factEditOpen.value = true;
  }

  async function submitFactEdit(payload: {
    statement: string;
    details_markdown: string;
    fact_kind: string;
    tags: string[];
  }) {
    if (factReviewActing.value) return;
    factReviewActing.value = true;
    const tags_json = JSON.stringify(payload.tags);
    try {
      if (factEditMode.value === 'refine') {
        const fact = selectedFact.value;
        if (!fact) return;
        const updated = await memoryStore.reviewFact({
          fact_id: fact.id,
          action: 'refine',
          statement: payload.statement,
          details_markdown: payload.details_markdown,
          fact_kind: payload.fact_kind,
          tags_json,
        });
        selectedFact.value = updated;
        notify({ type: 'positive', message: t('memory.factDrawer.reviewDone.refine') });
      } else {
        await memoryStore.upsertFact({
          scopeType: 'agent',
          scopeId: opts.selectedAgentId.value || undefined,
          agentId: opts.selectedAgentId.value || undefined,
          statement: payload.statement,
          detailsMarkdown: payload.details_markdown,
          factKind: payload.fact_kind || 'fact',
          tagsJson: tags_json,
          confidence: 0.85,
          sourceKind: 'manual',
          status: 'active',
        });
        notify({ type: 'positive', message: t('memory.factDrawer.createDone') });
      }
      factEditOpen.value = false;
      await loadFacts();
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.factDrawer.reviewFailed') });
    } finally {
      factReviewActing.value = false;
    }
  }

  function formatDate(value?: string) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }

  return {
    facts,
    selectedFact,
    factKeyword,
    factScope,
    factStatus,
    loadingFacts,
    factsEndpointReady,
    factsTotal,
    factsActiveCount,
    factsArchivedCount,
    factsFilteredCount,
    factPage,
    factPageSize,
    factPageMax,
    factDrawer,
    factEditOpen,
    factEditMode,
    factReviewActing,
    scopeOptions,
    factStatusOptions,
    factColumns,
    loadFacts,
    reloadFactsFromFirstPage,
    resetFactFilters,
    openFact,
    reviewSelectedFact,
    openRefineFact,
    openCreateFact,
    submitFactEdit,
    formatDate,
  };
}
