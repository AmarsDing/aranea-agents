import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../agents/types';
import type { Session } from '../session/types';
import type {
  AgentIdentity,
  AgentStrategyProfile,
  CascadeProposal,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  FactReviewAction,
  L0AssemblySnapshot,
  L1Task,
  MemoryFact,
} from './types';
import { buildMemoryAssemblyTableColumns, buildMemoryFactTableColumns } from './memoryTableUi';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useSessionStore } from '../../stores/session';
import { useMemoryStore } from '../../stores/memory';

export function useMemoryCenterPage() {
  const { notify } = useQuasar();
  const { t } = useI18n();
  const agentsCatalog = useAgentsCatalogStore();
  const sessionStore = useSessionStore();
  const memoryStore = useMemoryStore();
  const {
    facts: storeFacts,
    snapshots: storeSnapshots,
    entities: storeEntities,
    cascadePreview,
    cascadeSagaSteps,
    cascadeProposals: storeCascadeProposals,
    loadingCascade: storeLoadingCascade,
    loadingCascadePreview,
    loadingCascadeSaga,
  } = storeToRefs(memoryStore);

  const tab = ref('panorama');
  const agents = ref<Agent[]>([]);
  const sessions = ref<Session[]>([]);
  const facts = storeFacts;
  const entities = storeEntities;
  const evolutionProposals = ref<EvolutionProposal[]>([]);
  const cascadeProposals = storeCascadeProposals;
  const evolutionEvents = ref<EvolutionEvent[]>([]);
  const agentIdentity = ref<AgentIdentity | null>(null);
  const agentStrategy = ref<AgentStrategyProfile | null>(null);
  const evolutionMetrics = ref<EvolutionMetricsReport | null>(null);
  const snapshots = storeSnapshots;
  const tasks = ref<L1Task[]>([]);
  const selectedAgentId = ref<string | null>(null);
  const selectedSessionId = ref<string | null>(null);
  const selectedSnapshot = ref<L0AssemblySnapshot | null>(null);
  const selectedFact = ref<MemoryFact | null>(null);
  const factKeyword = ref('');
  const factScope = ref<string | null>(null);
  const factStatus = ref<string | null>('active');
  const loadingAgents = ref(false);
  const loadingSessions = ref(false);
  const loadingFacts = ref(false);
  const loadingSnapshots = ref(false);
  const loadingTasks = ref(false);
  const loadingEvolution = ref(false);
  const loadingCascade = storeLoadingCascade;
  const cascadeActingId = ref<string | null>(null);
  const factsEndpointReady = ref(true);
  const factsTotal = ref(0);
  const factsActiveCount = ref(0);
  const factsArchivedCount = ref(0);
  /** 与当前 status 过滤同口径的事实总数（filtered_count），驱动知识面板服务端分页。 */
  const factsFilteredCount = ref(0);
  const factPage = ref(1);
  const factPageSize = ref(10);
  const factPageMax = computed(() => Math.max(1, Math.ceil(factsFilteredCount.value / factPageSize.value)));
  // 请求序号守卫：翻页/筛选叠加时丢弃过期响应，避免旧数据覆盖新结果（对齐 useToolRunsPage 模式）。
  let factsLoadSeq = 0;
  const snapshotDrawer = ref(false);
  const factDrawer = ref(false);
  const factEditOpen = ref(false);
  const factEditMode = ref<'refine' | 'create'>('refine');
  const factReviewActing = ref(false);
  const conflictFacts = ref<MemoryFact[]>([]);
  const loadingConflicts = ref(false);
  const conflictActingId = ref<string | null>(null);
  const cascadePreviewOpen = ref(false);
  const cascadeSagaDrawerOpen = ref(false);
  const cascadePreviewProposalId = ref<string | null>(null);
  const cascadeSagaProposalId = ref<string | null>(null);
  const error = ref('');
  const workerStatus = computed(() => memoryStore.workerStatus);
  const loadingWorkerStatus = computed(() => memoryStore.loadingWorkerStatus);

  const loading = computed(
    () =>
      loadingAgents.value ||
      loadingSessions.value ||
      loadingFacts.value ||
      loadingSnapshots.value ||
      loadingTasks.value ||
      loadingEvolution.value ||
      loadingCascade.value,
  );
  const agentOptions = computed(() =>
    agents.value.map((agent) => ({ label: agent.display_name || agent.agent_key, value: agent.id })),
  );

  const overviewCards = computed(() => {
    const sessionSample = sessions.value.length;
    const avgContext = sessionSample
      ? sessions.value.reduce((sum, session) => sum + (session.context_used_ratio || 0), 0) / sessionSample
      : 0;
    const riskySessions = sessions.value.filter((session) =>
      ['warning', 'critical', 'exceeded'].includes(session.context_status),
    ).length;
    const activeTasks = tasks.value.filter((task) => task.status === 'active' || task.status === 'paused').length;
    return [
      {
        label: t('memory.metrics.contextRisk'),
        value: riskySessions,
        hint: t('memory.metrics.avgUsage', { percent: formatPercent(avgContext) }),
        icon: 'speed',
        color: contextRatioColor(avgContext),
      },
      {
        label: t('memory.metrics.activeTasks'),
        value: activeTasks,
        hint: t('memory.metrics.l1TasksHint'),
        icon: 'assignment',
        color: 'primary',
      },
      {
        label: t('memory.metrics.longTermKnowledge'),
        value: factsTotal.value,
        hint: factsEndpointReady.value ? t('memory.metrics.l3FactsTotal') : t('memory.metrics.l3FactsUnavailable'),
        icon: 'psychology',
        color: 'deep-purple',
      },
      {
        label: t('memory.metrics.graphEntities'),
        value: entities.value.length,
        hint: t('memory.metrics.l4EntitiesHint'),
        icon: 'device_hub',
        color: 'teal',
      },
      {
        label: t('memory.metrics.promptSnapshots'),
        value: snapshots.value.length,
        hint: t('memory.metrics.l0SnapshotsHint'),
        icon: 'preview',
        color: 'blue-grey',
      },
    ];
  });

  const evolutionPanels = computed(() => [
    {
      title: t('memory.evolution.graphTitle'),
      caption: t('memory.evolution.graphCaption'),
      state: t('memory.evolution.entitiesLoaded', { count: entities.value.length }),
      icon: 'device_hub',
      color: 'teal',
      items: entities.value
        .slice(0, 5)
        .map((entity) => `${entity.name} · ${entity.entity_type} · ${entity.scope_type}`),
    },
    {
      title: t('memory.evolution.identityTitle'),
      caption: t('memory.evolution.identityCaption'),
      state: agentIdentity.value
        ? t('memory.evolution.identityState', {
            phase: agentIdentity.value.current_phase || 'active',
            tone: agentIdentity.value.tone || 'tone unset',
          })
        : t('memory.evolution.identityEmpty'),
      icon: 'badge',
      color: 'primary',
      items: agentIdentity.value
        ? [
            agentIdentity.value.persona || t('memory.evolution.personaEmpty'),
            ...(agentIdentity.value.domains || [])
              .slice(0, 4)
              .map((domain) => t('memory.evolution.domainLabel', { domain })),
          ]
        : [],
    },
    {
      title: t('memory.evolution.strategyTitle'),
      caption: t('memory.evolution.strategyCaption'),
      state: agentStrategy.value
        ? t('memory.evolution.strategyState', {
            exploration: formatScore(agentStrategy.value.exploration),
            caution: formatScore(agentStrategy.value.caution),
          })
        : t('memory.evolution.strategyEmpty'),
      icon: 'tune',
      color: 'deep-purple',
      items: agentStrategy.value
        ? [
            t('memory.evolution.concisenessLabel', { value: formatScore(agentStrategy.value.conciseness) }),
            t('memory.evolution.delegationLabel', { value: formatScore(agentStrategy.value.delegation) }),
            t('memory.evolution.blacklistLabel', {
              value: (agentStrategy.value.tool_blacklist || []).join(', ') || t('memory.evolution.blacklistEmpty'),
            }),
          ]
        : [],
    },
    {
      title: t('memory.evolution.proposalsTitle'),
      caption: t('memory.evolution.proposalsCaption'),
      state: t('memory.evolution.proposalsState', {
        pending: evolutionProposals.value.length,
        events: evolutionMetrics.value?.events_total ?? evolutionEvents.value.length,
      }),
      icon: 'rule',
      color: 'orange',
      items: evolutionProposals.value
        .slice(0, 5)
        .map(
          (proposal) =>
            `${proposal.target_field}: ${proposal.rationale || proposal.expected_impact || proposal.status}`,
        ),
    },
  ]);

  const settingChecklist = computed(() => [
    { label: t('memory.checklist.basicSettings'), caption: t('memory.checklist.basicSettingsCaption'), done: true },
    { label: t('memory.checklist.l0Strategy'), caption: t('memory.checklist.l0StrategyCaption'), done: true },
    { label: t('memory.checklist.l1Budget'), caption: t('memory.checklist.l1BudgetCaption'), done: true },
    {
      label: t('memory.checklist.l3Semantic'),
      caption: t('memory.checklist.l3SemanticCaption'),
      done: factsEndpointReady.value,
    },
    { label: t('memory.checklist.workerModel'), caption: t('memory.checklist.workerModelCaption'), done: true },
    {
      label: t('memory.checklist.platformPolicy'),
      caption: t('memory.checklist.platformPolicyCaption'),
      done: true,
    },
    { label: t('memory.checklist.l4Graph'), caption: t('memory.checklist.l4GraphCaption'), done: true },
  ]);

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
  const snapshotColumns = computed(() => buildMemoryAssemblyTableColumns(formatDate, t));

  onMounted(loadAll);

  // Agent 变更的统一加载入口（含首挂载 loadAgents 回填首个 Agent 的场景）；
  // loadAll 仅在 Agent 未变化（手动刷新）时显式加载，避免 watcher + 显式调用双触发重复请求。
  watch(selectedAgentId, async () => {
    selectedSessionId.value = null;
    try {
      await Promise.all([loadSessions(), reloadFactsFromFirstPage(), loadEvolution(), loadCascade()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.error.loadFailed');
    }
  });

  watch(selectedSessionId, () => {
    void loadSessionMemory();
  });

  // 单 watch 合并分页：pageSize 变化先归一到第 1 页（page 变化复用同一 watch），避免越界空页与双请求。
  watch([factPage, factPageSize], (newVals, oldVals) => {
    const sizeChanged = newVals[1] !== oldVals[1];
    if (sizeChanged && factPage.value !== 1) {
      factPage.value = 1;
      return;
    }
    void loadFacts();
  });

  async function loadAll() {
    error.value = '';
    try {
      const prevAgentId = selectedAgentId.value;
      await loadAgents();
      if (selectedAgentId.value === prevAgentId) {
        await Promise.all([loadSessions(), reloadFactsFromFirstPage(), loadEvolution(), loadCascade()]);
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('memory.error.loadFailed');
    }
  }

  async function loadAgents() {
    loadingAgents.value = true;
    try {
      agents.value = await agentsCatalog.fetchAgents({ limit: 200 });
      if (!selectedAgentId.value && agents.value.length) {
        selectedAgentId.value = agents.value[0].id;
      }
    } finally {
      loadingAgents.value = false;
    }
  }

  async function loadSessions() {
    loadingSessions.value = true;
    try {
      await sessionStore.loadSessions({ agent_id: selectedAgentId.value || undefined, limit: 30 });
      sessions.value = sessionStore.sessions;
      if (!selectedSessionId.value && sessions.value.length) {
        selectedSessionId.value = sessions.value[0].id;
      }
    } finally {
      loadingSessions.value = false;
    }
  }

  async function loadCascade() {
    const agentID = selectedAgentId.value || agents.value[0]?.id || '';
    if (!agentID) {
      return;
    }
    await memoryStore.loadCascadeProposals(agentID);
  }

  async function approveCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.approveCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeApproveFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function rejectCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.rejectCascade(row.id);
      await loadCascade();
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeRejectFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function previewCascade(row: CascadeProposal) {
    cascadePreviewProposalId.value = row.id;
    cascadePreviewOpen.value = true;
    memoryStore.clearCascadePreview();
    await memoryStore.loadCascadePreview(row.id);
  }

  async function openSagaDrawer(row: CascadeProposal) {
    cascadeSagaProposalId.value = row.id;
    cascadeSagaDrawerOpen.value = true;
    await memoryStore.loadCascadeSagaSteps(row.id);
  }

  async function retryCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.retryCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeRetryFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function compensateCascade(row: CascadeProposal) {
    cascadeActingId.value = row.id;
    try {
      await memoryStore.compensateCascade(row.id);
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.error.cascadeCompensateFailed') });
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function confirmPreviewCascade(proposalId: string) {
    cascadeActingId.value = proposalId;
    try {
      await memoryStore.approveCascade(proposalId);
      cascadePreviewOpen.value = false;
      await Promise.all([loadCascade(), loadFacts(), loadEvolution()]);
    } finally {
      cascadeActingId.value = null;
    }
  }

  async function loadEvolution() {
    loadingEvolution.value = true;
    try {
      const agentID = selectedAgentId.value || agents.value[0]?.id || '';
      const bundle = await memoryStore.loadEvolutionForAgent(agentID);
      agentIdentity.value = bundle.identity;
      agentStrategy.value = bundle.strategy;
      evolutionProposals.value = bundle.proposals;
      evolutionEvents.value = bundle.events;
      evolutionMetrics.value = bundle.metrics;
    } catch {
      memoryStore.clearEntities();
      agentIdentity.value = null;
      agentStrategy.value = null;
      evolutionProposals.value = [];
      evolutionEvents.value = [];
      evolutionMetrics.value = null;
    } finally {
      loadingEvolution.value = false;
    }
  }

  async function loadConflictingFacts() {
    const agentID = selectedAgentId.value || agents.value[0]?.id || '';
    if (!agentID) {
      conflictFacts.value = [];
      return;
    }
    loadingConflicts.value = true;
    try {
      // H2: agent_id 跨 scope 口径——冲突事实分布在 session/user/agent scope，
      // 仅查 agent scope 会漏计（与 F1 facts 列表口径一致）。
      const result = await memoryStore.loadConflictingFacts('', '', agentID, 50, 0);
      conflictFacts.value = result.items;
    } catch {
      // 冲突列表拉取失败不阻断 facts 主列表。
      conflictFacts.value = [];
    } finally {
      loadingConflicts.value = false;
    }
  }

  /** 冲突仲裁：confirm/reject/deprecate 后刷新冲突列表与 facts 主列表。 */
  async function reviewConflictFact(fact: MemoryFact, action: FactReviewAction) {
    if (conflictActingId.value) return;
    conflictActingId.value = fact.id;
    try {
      await memoryStore.reviewFact({ fact_id: fact.id, action });
      notify({ type: 'positive', message: t(`memory.factDrawer.reviewDone.${action}`) });
      await Promise.all([loadConflictingFacts(), loadFacts()]);
    } catch (e: unknown) {
      notify({ type: 'negative', message: e instanceof Error ? e.message : t('memory.factDrawer.reviewFailed') });
    } finally {
      conflictActingId.value = null;
    }
  }

  async function loadFacts() {
    const seq = ++factsLoadSeq;
    loadingFacts.value = true;
    try {
      const result = await memoryStore.loadFacts({
        keyword: factKeyword.value || undefined,
        scope_type: factScope.value || undefined,
        status: factStatus.value || undefined,
        agent_id: selectedAgentId.value || undefined,
        limit: factPageSize.value,
        offset: (factPage.value - 1) * factPageSize.value,
      });
      if (seq !== factsLoadSeq) return;
      factsTotal.value = result.total;
      factsFilteredCount.value = result.filtered_count ?? result.total;
      factsActiveCount.value = result.active_count ?? 0;
      factsArchivedCount.value = result.archived_count ?? 0;
      factsEndpointReady.value = true;
      // 治理动作删减事实后当前页可能越界：归一到最新末页，由 page watch 复用本函数重新加载。
      if (factPage.value > factPageMax.value) {
        factPage.value = factPageMax.value;
        return;
      }
      await loadConflictingFacts();
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

  /** 筛选/搜索/Agent 变更后从第 1 页重新加载：page 已是 1 时直接加载，>1 时归一由 page watch 触发，避免双请求。 */
  async function reloadFactsFromFirstPage() {
    if (factPage.value !== 1) {
      factPage.value = 1;
      return;
    }
    await loadFacts();
  }

  async function loadSessionMemory() {
    if (!selectedSessionId.value) {
      memoryStore.clearSnapshots();
      tasks.value = [];
      return;
    }
    await Promise.all([loadSnapshots(), loadTasks()]);
  }

  async function loadSnapshots() {
    if (!selectedSessionId.value) return;
    loadingSnapshots.value = true;
    try {
      await memoryStore.loadSnapshots(selectedSessionId.value, 20);
    } catch {
      memoryStore.clearSnapshots();
    } finally {
      loadingSnapshots.value = false;
    }
  }

  async function loadTasks() {
    if (!selectedSessionId.value) return;
    loadingTasks.value = true;
    try {
      tasks.value = await memoryStore.loadL1Tasks(selectedSessionId.value, { include_ended: true });
    } catch {
      tasks.value = [];
    } finally {
      loadingTasks.value = false;
    }
  }

  function resetFactFilters() {
    factKeyword.value = '';
    factScope.value = null;
    factStatus.value = 'active';
    void reloadFactsFromFirstPage();
  }

  function openSnapshot(row: L0AssemblySnapshot) {
    selectedSnapshot.value = row;
    snapshotDrawer.value = true;
  }

  function openFact(row: MemoryFact) {
    selectedFact.value = row;
    factDrawer.value = true;
  }

  /** 抽屉治理动作：confirm/reject 就地更新；archive/deprecate 变更状态后按当前过滤刷新列表。 */
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

  /** 编辑对话框提交：refine 走列级 ReviewMemoryFact；create 走 UpsertMemoryFact（默认 agent 作用域）。 */
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
        // agentId 必须显式传入：列表/统计走 F1 口径（memory_facts.agent_id 列，
        // 按产生方 agent 跨 scope 聚合），仅设 scope=agent 不带 agent_id 会导致
        // 新事实在当前 Agent 过滤下不可见。
        await memoryStore.upsertFact({
          scopeType: 'agent',
          scopeId: selectedAgentId.value || undefined,
          agentId: selectedAgentId.value || undefined,
          statement: payload.statement,
          detailsMarkdown: payload.details_markdown,
          factKind: payload.fact_kind || 'fact',
          tagsJson: tags_json,
          // 用户手动陈述的事实给中高默认置信度（对齐 auto_memory 的 0.85 量级），
          // 缺省 0 会在列表中显示红色 0%，与「用户明确告知」的语义相悖。
          confidence: 0.85,
          // 手动创建标记来源，区别于 auto_memory 自动提炼，便于治理时辨识。
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

  function contextRatioColor(value?: number) {
    const ratio = Math.max(0, Math.min(1, Number(value) || 0));
    if (ratio >= 0.85) return 'negative';
    if (ratio >= 0.6) return 'warning';
    return 'positive';
  }

  function formatPercent(value?: number) {
    return `${Math.round((Number(value) || 0) * 100)}%`;
  }

  function formatScore(value?: number) {
    return (Number(value) || 0).toFixed(2);
  }

  function formatDate(value?: string) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }

  async function handleDeadLetterReplay(id: number) {
    await memoryStore.replayDeadLetter(id);
  }

  async function handleDeadLetterAbandon(id: number) {
    await memoryStore.abandonDeadLetter(id);
  }

  async function loadWorkerStatus() {
    await memoryStore.loadWorkerStatus();
  }

  return {
    tab,
    selectedAgentId,
    selectedSessionId,
    selectedSnapshot,
    selectedFact,
    factKeyword,
    factScope,
    factStatus,
    snapshotDrawer,
    factDrawer,
    factEditOpen,
    factEditMode,
    factReviewActing,
    conflictFacts,
    loadingConflicts,
    conflictActingId,
    error,
    loading,
    loadingFacts,
    loadingSessions,
    loadingSnapshots,
    loadingTasks,
    agentOptions,
    sessionRows: sessions,
    factRows: facts,
    snapshotRows: snapshots,
    taskRows: tasks,
    overviewCards,
    evolutionPanels,
    entities,
    loadingEvolution,
    cascadeProposals,
    loadingCascade,
    cascadeActingId,
    cascadePreviewOpen,
    cascadePreviewData: cascadePreview,
    cascadePreviewProposalId,
    loadingCascadePreview,
    cascadeSagaDrawerOpen,
    cascadeSagaProposalId,
    sagaSteps: cascadeSagaSteps,
    loadingCascadeSaga,
    settingChecklist,
    scopeOptions,
    factStatusOptions,
    factColumns,
    snapshotColumns,
    factsEndpointReady,
    factsActiveCount,
    factsArchivedCount,
    factsFilteredCount,
    factPage,
    factPageSize,
    factPageMax,
    loadAll,
    loadSessions,
    loadFacts,
    reloadFactsFromFirstPage,
    loadSessionMemory,
    loadCascade,
    approveCascade,
    rejectCascade,
    previewCascade,
    openSagaDrawer,
    retryCascade,
    compensateCascade,
    confirmPreviewCascade,
    resetFactFilters,
    openSnapshot,
    openFact,
    reviewSelectedFact,
    openRefineFact,
    openCreateFact,
    submitFactEdit,
    reviewConflictFact,
    loadEvolution,
    handleDeadLetterReplay,
    handleDeadLetterAbandon,
    workerStatus,
    loadingWorkerStatus,
    loadWorkerStatus,
  };
}
