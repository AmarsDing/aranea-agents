import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import type { Agent } from '../agents/types';
import type { Session } from '../session/types';
import type {
  AgentIdentity,
  AgentStrategyProfile,
  CascadeProposal,
  EvolutionEvent,
  EvolutionMetricsReport,
  EvolutionProposal,
  L0AssemblySnapshot,
  L1Task,
  MemoryFact,
} from './types';
import { buildMemoryAssemblyTableColumns, buildMemoryFactTableColumns } from './memoryTableUi';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useSessionStore } from '../../stores/session';
import { useMemoryStore } from '../../stores/memory';
import { MEMORY_ENTITY_LIMIT } from '../constants/queryLimits';

export function useMemoryCenterPage() {
  const { notify } = useQuasar();
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

  const tab = ref('overview');
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
  const conflictingFactsTotal = ref(0);
  const snapshotDrawer = ref(false);
  const factDrawer = ref(false);
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
        label: '上下文风险',
        value: riskySessions,
        hint: `样本≤30 会话 · 平均占用 ${formatPercent(avgContext)}`,
        icon: 'speed',
        color: contextRatioColor(avgContext),
      },
      { label: '活跃任务', value: activeTasks, hint: 'L1 working memory tasks', icon: 'assignment', color: 'primary' },
      {
        label: '长期知识',
        value: factsTotal.value,
        hint: factsEndpointReady.value ? '服务端 facts 总数' : 'L3 facts 暂不可用',
        icon: 'psychology',
        color: 'deep-purple',
      },
      {
        label: '图谱实体',
        value: entities.value.length,
        hint: `已加载样本（上限 ${MEMORY_ENTITY_LIMIT}）`,
        icon: 'device_hub',
        color: 'teal',
      },
      {
        label: 'Prompt 快照',
        value: snapshots.value.length,
        hint: '最近 L0 assembly snapshots',
        icon: 'preview',
        color: 'blue-grey',
      },
    ];
  });

  const actionItems = computed(() => [
    {
      title: '上下文接近上限',
      caption: '基于已加载会话样本（≤30）；建议检查摘要阈值和注入片段数量。',
      count: sessions.value.filter((s) => ['warning', 'critical', 'exceeded'].includes(s.context_status)).length,
      icon: 'report',
      color: 'warning',
    },
    {
      title: '知识冲突待办',
      caption: 'ListConflictingFacts 服务端总数（当前 Agent scope）。',
      count: conflictingFactsTotal.value,
      icon: 'rule',
      color: 'negative',
    },
    {
      title: '待审核进化提议',
      caption: '来自 Agent Evolution proposal queue。',
      count: evolutionProposals.value.length,
      icon: 'auto_awesome',
      color: 'info',
    },
    {
      title: 'Cascade 更名待审',
      caption: 'L4 冲突门控产生的图谱/L3 级联审核。',
      count: cascadeProposals.value.filter((p) => p.status === 'pending').length,
      icon: 'sync_alt',
      color: 'deep-orange',
    },
  ]);

  const memoryLayers = computed(() => [
    {
      key: 'l0',
      title: '上下文窗口 L0',
      caption: '下一次模型调用实际看到的材料。',
      icon: 'preview',
      color: 'primary',
      status: '已接入',
      statusColor: 'positive',
    },
    {
      key: 'l1',
      title: '工作记忆 L1',
      caption: '当前任务目标、约束、决策和中间结果。',
      icon: 'assignment',
      color: 'indigo',
      status: '已接入',
      statusColor: 'positive',
    },
    {
      key: 'l2',
      title: '事件记忆 L2',
      caption: '会话 timeline、episode、marks 与巩固队列。',
      icon: 'timeline',
      color: 'teal',
      status: '已接入',
      statusColor: 'positive',
    },
    {
      key: 'l3',
      title: '知识记忆 L3',
      caption: '跨会话 facts、偏好、规则、冲突与反馈。',
      icon: 'psychology',
      color: 'deep-purple',
      status: factsEndpointReady.value ? '已接入' : '不可用',
      statusColor: factsEndpointReady.value ? 'positive' : 'warning',
    },
    {
      key: 'l4',
      title: '图谱与进化 L4',
      caption: '实体关系、Agent identity、strategy 和 proposal。',
      icon: 'auto_awesome',
      color: 'orange',
      status: entities.value.length || agentIdentity.value ? '已接入' : '已注册',
      statusColor: 'positive',
    },
  ]);

  const evolutionPanels = computed(() => [
    {
      title: '知识图谱',
      caption: '实体、关系、证据链和邻居召回。',
      state: `${entities.value.length} 个实体已加载`,
      icon: 'device_hub',
      color: 'teal',
      items: entities.value
        .slice(0, 5)
        .map((entity) => `${entity.name} · ${entity.entity_type} · ${entity.scope_type}`),
    },
    {
      title: 'Agent Identity',
      caption: 'persona、values、tone、domains 和用户期望。',
      state: agentIdentity.value
        ? `${agentIdentity.value.current_phase || 'active'} · ${agentIdentity.value.tone || 'tone unset'}`
        : '选择 Agent 后加载 identity',
      icon: 'badge',
      color: 'primary',
      items: agentIdentity.value
        ? [
            agentIdentity.value.persona || 'Persona 尚未填写',
            ...(agentIdentity.value.domains || []).slice(0, 4).map((domain) => `Domain: ${domain}`),
          ]
        : [],
    },
    {
      title: 'Strategy Profile',
      caption: '探索度、简洁度、谨慎度、工具偏好和模型偏好。',
      state: agentStrategy.value
        ? `exploration=${formatScore(agentStrategy.value.exploration)} · caution=${formatScore(agentStrategy.value.caution)}`
        : '选择 Agent 后加载 strategy',
      icon: 'tune',
      color: 'deep-purple',
      items: agentStrategy.value
        ? [
            `conciseness=${formatScore(agentStrategy.value.conciseness)}`,
            `delegation=${formatScore(agentStrategy.value.delegation)}`,
            `blacklist=${(agentStrategy.value.tool_blacklist || []).join(', ') || 'empty'}`,
          ]
        : [],
    },
    {
      title: 'Evolution Proposals',
      caption: '待审核的自我修正建议和回滚日志。',
      state: `${evolutionProposals.value.length} pending · ${evolutionMetrics.value?.events_total ?? evolutionEvents.value.length} events`,
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
    { label: '基础 memory_* 设置', caption: 'Agent 设置页已有旧版记忆启用、结果数和最低分数。', done: true },
    { label: 'L0 上下文策略', caption: 'Prompt snapshot / preview API 已接入。', done: true },
    { label: 'L1 工作记忆预算', caption: 'L1 task/field API 已接入。', done: true },
    {
      label: 'L3 语义记忆设置',
      caption: 'Facts / recall：`memory/v1` 由 cmd/admin SQLite（sessionmemory）提供。',
      done: factsEndpointReady.value,
    },
    { label: '巩固 Worker 模型', caption: 'Agent 设置 → 记忆 Tab：`memory_worker_*` / `l0_compress_*`。', done: true },
    {
      label: '平台 Policy Strict / Backfill',
      caption: '记忆中心 → 设置 Tab：MEMORY_POLICY_STRICT / MEMORY_EPISODE_BACKFILL_DISABLED（DB + env）。',
      done: true,
    },
    { label: 'L4 图谱与进化设置', caption: 'Entities / neighborhood / evolution API 已注册并在本页读取。', done: true },
  ]);

  const scopeOptions = ['user', 'agent', 'team', 'workspace', 'global'].map((value) => ({ label: value, value }));
  const factStatusOptions = ['active', 'archived', 'disputed', 'deprecated', 'deleted'].map((value) => ({
    label: value,
    value,
  }));

  const factColumns = buildMemoryFactTableColumns(formatDate);
  const snapshotColumns = buildMemoryAssemblyTableColumns(formatDate);

  onMounted(loadAll);

  watch(selectedAgentId, async () => {
    selectedSessionId.value = null;
    await Promise.all([loadSessions(), loadEvolution(), loadCascade()]);
  });

  watch(selectedSessionId, () => {
    void loadSessionMemory();
  });

  async function loadAll() {
    error.value = '';
    try {
      await loadAgents();
      await Promise.all([loadSessions(), loadFacts(), loadEvolution(), loadCascade()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : '记忆中心加载失败';
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
      notify({ type: 'negative', message: e instanceof Error ? e.message : 'Cascade 批准失败' });
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
      notify({ type: 'negative', message: e instanceof Error ? e.message : 'Cascade 拒绝失败' });
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
      notify({ type: 'negative', message: e instanceof Error ? e.message : 'Cascade 重试失败' });
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
      notify({ type: 'negative', message: e instanceof Error ? e.message : 'Cascade 补偿失败' });
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
      conflictingFactsTotal.value = 0;
      return;
    }
    try {
      const result = await memoryStore.loadConflictingFacts('agent', agentID, 50, 0);
      conflictingFactsTotal.value = result.total;
    } catch {
      conflictingFactsTotal.value = 0;
    }
  }

  async function loadFacts() {
    loadingFacts.value = true;
    try {
      const result = await memoryStore.loadFacts({
        keyword: factKeyword.value || undefined,
        scope_type: factScope.value || undefined,
        status: factStatus.value || undefined,
        limit: 50,
      });
      factsTotal.value = result.total;
      factsEndpointReady.value = true;
      await loadConflictingFacts();
    } catch {
      memoryStore.clearFacts();
      factsTotal.value = 0;
      conflictingFactsTotal.value = 0;
      factsEndpointReady.value = false;
    } finally {
      loadingFacts.value = false;
    }
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
    void loadFacts();
  }

  function openSnapshot(row: L0AssemblySnapshot) {
    selectedSnapshot.value = row;
    snapshotDrawer.value = true;
  }

  function openFact(row: MemoryFact) {
    selectedFact.value = row;
    factDrawer.value = true;
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
    actionItems,
    memoryLayers,
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
    loadAll,
    loadSessions,
    loadFacts,
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
    loadEvolution,
    handleDeadLetterReplay,
    handleDeadLetterAbandon,
    workerStatus,
    loadingWorkerStatus,
    loadWorkerStatus,
  };
}
