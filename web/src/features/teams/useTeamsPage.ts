import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { i18n } from '../../i18n';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { Agent } from '../agents/types';
import type { Team, TeamDefinition, TeamRun, TeamRunEvent, TaskDeadLetterRow } from './types';
import { listTaskDeadLetters, resolveTaskDeadLetter } from './api';
import { useTeamsStore } from '../../stores/teams';
import { usePlatformStore } from '../../stores/platform';
import {
  definitionFromTemplate,
  definitionGraphFromCompileJSON,
  definitionToJSON,
  definitionTopologyKey,
  definitionTopologyOverwriteKey,
  deriveMemberRolesForMode,
  groupTeamsByDepartment,
  departmentOptionsFromTree,
  linkableGraphOptions,
  parseDefinition,
  rebuildDefinitionGraph,
  resetDefinition,
  validateTeamDefinition,
  type TeamTemplateKey,
} from '../../components/teams/teamUtils';
// TECH-DEBT(FL5): 同 useTeamCompilePreview —— compile RPC 为编辑器即时操作，
// 不落地全局状态；如需持久化编译结果应迁入 Store。
import { compileTeamGraph } from '../orchestration/compileApi';
import { useGraphStore } from '../../stores/graph';
import type { PlatformResourceTreeNode } from '../platform/types';

/**
 * useTeamsPage — thin orchestrator composing sub-concerns.
 * Data lives in useTeamsStore; UI transient state lives here.
 */
export function useTeamsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const store = useTeamsStore();
  const graphStore = useGraphStore();
  const isDark = computed(() => $q.dark.isActive);

  // Store refs (single source of truth for data)
  const {
    agents: storeAgents,
    runs,
    runsLoading,
    runsError,
    stepsByRun,
    stepsLoading,
    summariesByRun,
    summariesLoading,
    detailsByRun,
    detailsLoading,
  } = storeToRefs(store);

  // ── List state ──
  const loading = ref(false);
  const error = ref('');
  const search = ref('');
  const modeFilter = ref('');
  const statusFilter = ref('');
  const departmentFilter = ref('');
  // Orchestration-generated teams (spirit_session_id set) are hidden by default
  // on the management page; users opt in via the toolbar toggle.
  const showOrchestrated = ref(false);
  const taxonomyTree = ref<PlatformResourceTreeNode[]>([]);
  const currentPage = ref(1);
  const pageSize = ref(12);

  // ── Editor state ──
  const editorOpen = ref(false);
  const selectedTeamTemplateKey = ref<TeamTemplateKey | null>(null);
  const editingId = ref('');
  const editingHasActiveRun = ref(false);
  const saving = ref(false);
  const form = reactive({
    team_key: '',
    display_name: '',
    status: 'pending',
    app_name: '',
    taxonomy_industry_id: '',
    spirit_session_id: '',
  });
  const definition = reactive<TeamDefinition>({
    version: 1,
    description: '',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: [],
  });

  // ── ADR-08 A1 派生同步：拓扑字段变更 → 实时重建 embedded graph ──
  // 打开编辑器/应用模板时经 beginDefinitionSnapshot 建立拓扑基线；此后任何拓扑
  // 变更（mode / synthesizer / members 增删改启停排序）立即重建 definition.graph，
  // 使保存的 definition_json 中 graph 永远与 mode/members 一致（修 ADR-08 割裂点 1）。
  const savedTopologyKey = ref('');
  // M53 Phase 11 F2：打开/重置/模板应用时的覆盖确认基线（拓扑+checkpoint 指纹）。
  // 不随后续 watcher 推进——watcher 只更新 savedTopologyKey 用于去抖。
  const editorOverwriteBaselineKey = ref('');
  let applyingSnapshot = false;
  const topologyKey = computed(() => definitionTopologyKey(definition));
  watch(
    topologyKey,
    (key) => {
      if (applyingSnapshot || !editorOpen.value || key === savedTopologyKey.value) return;
      // A3：角色/synthesizer 由 mode+成员顺序派生（幂等）。派生改动会变更拓扑指纹，
      // 先把基线推到派生后指纹，抑制紧随的二次触发。
      const derived = deriveMemberRolesForMode(definition);
      // A1 不变量：同步本地重建，保证「保存时 graph 必新鲜」。
      definition.graph = rebuildDefinitionGraph(definition);
      savedTopologyKey.value = derived ? definitionTopologyKey(definition) : key;
      // A2：随后异步以后端 compile 的 canonical 图覆盖本地简图。
      scheduleGraphSyncFromBackend();
    },
    { flush: 'sync' },
  );

  /** 装载 definition（打开/重置/模板应用）期间暂停派生重建，并以装载后拓扑为基线。 */
  function beginDefinitionSnapshot(update: () => void) {
    applyingSnapshot = true;
    try {
      update();
    } finally {
      savedTopologyKey.value = definitionTopologyKey(definition);
      editorOverwriteBaselineKey.value = definitionTopologyOverwriteKey(definition);
      applyingSnapshot = false;
    }
  }

  // ── ADR-08 A2 模板去重：canonical graph 以后端 compile 为准，本地 builder 仅回退 ──
  // 拓扑变更后先同步本地重建（上方 watch），再 debounce 调 CompileTeamGraph 取
  // definition_graph_json 覆盖；seq 令牌防竞态（快速连续编辑时丢弃过期响应）；
  // 失败时本地简图已生效（graphUtils 降级为回退），每次编辑会话仅提示一次。
  let graphSyncTimer: ReturnType<typeof setTimeout> | null = null;
  let graphSyncSeq = 0;
  let graphSyncWarned = false;

  function scheduleGraphSyncFromBackend() {
    if (graphSyncTimer) clearTimeout(graphSyncTimer);
    const seq = ++graphSyncSeq;
    graphSyncTimer = setTimeout(() => void syncGraphFromBackend(seq), 400);
  }

  async function syncGraphFromBackend(seq: number) {
    const payload = JSON.parse(definitionToJSON(definition)) as Record<string, unknown>;
    // 剥离合 graph 强制后端走模板路径；否则 embedded graph 优先，只会回显旧图。
    delete payload.graph;
    try {
      const result = await compileTeamGraph(editingId.value.trim() || 'draft-preview', JSON.stringify(payload));
      if (seq !== graphSyncSeq || !editorOpen.value) return;
      const next = definitionGraphFromCompileJSON(result.definition_graph_json, definition.graph);
      if (next) definition.graph = next;
    } catch {
      if (seq !== graphSyncSeq || !editorOpen.value || graphSyncWarned) return;
      graphSyncWarned = true;
      $q.notify({ type: 'warning', message: '后端编排编译不可用，已回退为本地简图' });
    }
  }

  // ── Runs UI state ──
  const runsOpen = ref(false);
  // UI-2：连接状态派生自 WS 流 connected ref（订阅建立/断开即时反映），
  // 不再等首个事件置位——空闲 Team 不再恒显示「未连接」。
  // shallowRef 持有流句柄：深响应式会解包内部 connected ref，导致派生失效。
  const runEventsSource = shallowRef<ReturnType<typeof store.subscribeRunEvents> | null>(null);
  const runEventsConnected = computed(() => runEventsSource.value?.connected.value ?? false);
  const deadLetters = ref<TaskDeadLetterRow[]>([]);
  const deadLettersLoading = ref(false);

  // ── Test state ──
  const testOpen = ref(false);
  const testTeam = ref<Team | null>(null);
  const testLoading = ref(false);
  const testError = ref('');
  const testReply = ref('');
  const testRun = ref<TeamRun | null>(null);

  // ── Derived ──
  const agentOptions = computed(() =>
    store.agents.map((agent: Agent) => ({ label: agent.display_name, value: agent.id })),
  );
  // M53 Phase 11 F2：「关联 Graph」选择器仅列独立图资产（排除 team-owned）。
  const graphOptions = computed(() => linkableGraphOptions(graphStore.graphs));
  const definitionJSON = computed(() => definitionToJSON(definition));
  const canSave = computed(() =>
    Boolean(form.team_key && form.display_name && definition.members.some((member) => member.enabled)),
  );
  const matchesListFilters = (team: Team) => {
    const q = search.value.trim().toLowerCase();
    const def = parseDefinition(team);
    const matchesSearch =
      !q ||
      [team.display_name, team.team_key, def.description].some((value) => (value || '').toLowerCase().includes(q));
    const matchesMode = !modeFilter.value || def.mode === modeFilter.value;
    const matchesStatus = !statusFilter.value || team.status === statusFilter.value;
    return matchesSearch && matchesMode && matchesStatus;
  };
  const isOrchestrated = (team: Team) => String(team.spirit_session_id || '').trim() !== '';
  const filteredTeams = computed(() =>
    store.teams.filter((team) => (showOrchestrated.value || !isOrchestrated(team)) && matchesListFilters(team)),
  );
  // Orchestrated teams hidden solely by the toggle — surfaced in the empty state
  // so users don't mistake "filtered out" for "no teams exist".
  const hiddenOrchestratedCount = computed(() =>
    showOrchestrated.value ? 0 : store.teams.filter((team) => isOrchestrated(team) && matchesListFilters(team)).length,
  );
  const departmentOptions = computed(() => departmentOptionsFromTree(taxonomyTree.value));
  const totalFiltered = computed(() => filteredTeams.value.length);
  const pageMax = computed(() => Math.max(1, Math.ceil(totalFiltered.value / pageSize.value)));
  const paginatedTeams = computed(() => {
    const start = (currentPage.value - 1) * pageSize.value;
    return filteredTeams.value.slice(start, start + pageSize.value);
  });
  const teamGroups = computed(() =>
    groupTeamsByDepartment(paginatedTeams.value, store.agents, taxonomyTree.value, departmentFilter.value),
  );

  watch([search, modeFilter, statusFilter, departmentFilter, showOrchestrated], () => {
    currentPage.value = 1;
  });

  onMounted(loadRows);
  onBeforeUnmount(() => {
    closeRunEvents();
    if (graphSyncTimer) clearTimeout(graphSyncTimer);
    graphSyncSeq++; // 使在途的 graph 同步响应失效
  });
  watch(
    () => route.query.edit,
    () => openRouteEdit(),
  );
  watch(runsOpen, (open) => {
    if (!open) closeRunEvents();
  });

  // ── List actions ──

  async function loadRows() {
    loading.value = true;
    error.value = '';
    try {
      const platformStore = usePlatformStore();
      // Backend Team.department_id references organization tree nodes (not legacy taxonomy).
      await Promise.all([store.loadTeams(), store.loadAgents(), platformStore.loadTaxonomyTree('organization')]);
      taxonomyTree.value = platformStore.taxonomyTree;
      openRouteEdit();
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 Team 失败';
    } finally {
      loading.value = false;
    }
  }

  function openRouteEdit() {
    const editID = typeof route.query.edit === 'string' ? route.query.edit : '';
    if (!editID || !store.teams.length) return;
    const team = store.teams.find((row) => row.id === editID);
    if (team) openEdit(team);
  }

  // ── Editor actions ──

  /** F2：编辑器打开时加载图资产列表（「关联 Graph」选择器数据源，幂等刷新）。 */
  function loadGraphsForEditor() {
    void graphStore.loadGraphs(200).catch(() => {
      /* 列表加载失败不阻塞编辑；选择器为空即可 */
    });
  }

  function openCreate() {
    editingId.value = '';
    editingHasActiveRun.value = false;
    selectedTeamTemplateKey.value = null;
    graphSyncWarned = false;
    Object.assign(form, {
      team_key: '',
      display_name: '',
      status: 'pending',
      app_name: '',
      taxonomy_industry_id: '',
      spirit_session_id: '',
    });
    beginDefinitionSnapshot(() => resetDefinition(definition));
    loadGraphsForEditor();
    editorOpen.value = true;
  }

  function openEdit(team: Team) {
    editingId.value = team.id;
    editingHasActiveRun.value = Boolean(team.has_active_run);
    selectedTeamTemplateKey.value = null;
    Object.assign(form, {
      team_key: team.team_key,
      display_name: team.display_name,
      status: team.status,
      app_name: team.app_name,
      taxonomy_industry_id: team.taxonomy_industry_id || '',
      spirit_session_id: team.spirit_session_id || '',
    });
    beginDefinitionSnapshot(() => {
      Object.assign(definition, parseDefinition(team));
      // ADR-08 A3：编辑器已移除 runtime_engine 选项，保存统一走 Graph 运行时；
      // 遗留 native 定义在编辑器中打开即归一为 graph（native 仅供编排页 admin 调试）。
      definition.runtime_engine = 'graph';
      definition.team_graph_runtime = true;
    });
    loadGraphsForEditor();
    editorOpen.value = true;
  }

  /**
   * M53 Phase 11 F2：custom → 重置为派生。放弃 Graph 编辑器自定义拓扑，
   * 按当前表单字段重建本地图并回基线；保存后后端按 preset 物化（原地更新 owned 资产）。
   */
  function resetToDerived() {
    beginDefinitionSnapshot(() => {
      definition.source = 'preset';
      definition.graph = rebuildDefinitionGraph(definition);
    });
    scheduleGraphSyncFromBackend();
  }

  function addMember() {
    definition.members.push({
      agent_id: store.agents[0]?.id ?? '',
      role: 'worker',
      name: store.agents[0]?.display_name ?? '',
      enabled: true,
      sort_order: (definition.members.length + 1) * 10,
    });
  }

  function removeMember(index: number) {
    definition.members.splice(index, 1);
  }

  function applyTemplate(template: TeamTemplateKey) {
    if (store.agents.length === 0) {
      $q.notify({ type: 'warning', message: '请先创建或加载 Agent 后再应用模板' });
      selectedTeamTemplateKey.value = null;
      return;
    }
    beginDefinitionSnapshot(() => {
      resetDefinition(definition);
      Object.assign(definition, definitionFromTemplate(template, store.agents));
    });
    // 模板应用不走 topology watch（基线已重置），显式触发后端 canonical 图同步。
    scheduleGraphSyncFromBackend();
    $q.notify({ type: 'positive', message: 'Team 模板已应用' });
  }

  async function save() {
    const hint = validateTeamDefinition(definition);
    if (hint) {
      $q.notify({ type: 'warning', message: hint });
      return;
    }
    saving.value = true;
    try {
      const payload = {
        team_key: form.team_key,
        display_name: form.display_name,
        status: form.status,
        app_name: form.app_name || form.team_key,
        definition_json: definitionJSON.value,
        taxonomy_industry_id: form.taxonomy_industry_id || '',
      };
      await (editingId.value ? store.editTeam(editingId.value, payload) : store.addTeam(payload));
      editorOpen.value = false;
      $q.notify({ type: 'positive', message: 'Team 已保存' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
    } finally {
      saving.value = false;
    }
  }

  async function duplicate(team: Team) {
    try {
      await store.copy(team.id);
      $q.notify({ type: 'positive', message: 'Team 已复制' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '复制失败' });
    }
  }

  function confirmRemove(team: Team) {
    $q.dialog({
      title: '删除 Team',
      message: `确定删除「${team.display_name}」？此操作不可撤销。`,
      cancel: true,
      persistent: true,
    }).onOk(() => void remove(team));
  }

  async function remove(team: Team) {
    try {
      await store.remove(team.id);
      $q.notify({ type: 'info', message: 'Team 已删除' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    }
  }

  // ── Runs actions ──

  async function openRuns(team: Team) {
    store.selectedTeam = team;
    runsOpen.value = true;
    store.clearRunsState();
    await Promise.all([store.loadRuns(team.id, 30), loadDeadLetters()]);
    openRunEvents(team.id);
  }

  async function loadRuns() {
    if (!store.selectedTeam) return;
    await store.loadRuns(store.selectedTeam.id, 30);
  }

  async function loadDeadLetters() {
    if (!store.selectedTeam) return;
    deadLettersLoading.value = true;
    try {
      deadLetters.value = await listTaskDeadLetters({
        teamId: store.selectedTeam.id,
        status: 'pending',
        limit: 50,
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '加载死信失败' });
      deadLetters.value = [];
    } finally {
      deadLettersLoading.value = false;
    }
  }

  async function resolveDeadLetter(id: string) {
    try {
      await resolveTaskDeadLetter(id);
      $q.notify({ type: 'positive', message: '死信已标记为已解决' });
      await loadDeadLetters();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '解决死信失败' });
    }
  }

  async function loadRunSteps(runID: string) {
    await store.loadRunSteps(runID);
  }

  async function loadRunSummary(runID: string) {
    try {
      await store.loadRunSummary(runID);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '加载汇总失败' });
    }
  }

  // 79-runtime-governance 0.1：单 run 详情（cache_hit_ratio 载体）。命中率是增强
  // 字段，加载失败不打断展开交互，仅 notify。
  async function loadRunDetail(runID: string) {
    try {
      await store.loadRunDetail(runID);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '加载运行详情失败' });
    }
  }

  function openRunEvents(teamID: string) {
    closeRunEvents();
    runEventsSource.value = store.subscribeRunEvents(GLOBAL_WS_SESSION_ID, teamID, applyRunEvent);
  }

  function closeRunEvents() {
    runEventsSource.value?.close();
    runEventsSource.value = null;
  }

  function applyRunEvent(event: TeamRunEvent) {
    if (store.selectedTeam && event.team_id !== store.selectedTeam.id) return;
    if (event.run) store.upsertRun(event.run);
    if (event.step) store.upsertRunStep(event.step);
  }

  // ── Test actions ──

  function openRunTest(team: Team) {
    testTeam.value = team;
    testError.value = '';
    testReply.value = '';
    testRun.value = null;
    testOpen.value = true;
  }

  async function executeRunTest(content: string) {
    if (!testTeam.value) return;
    testLoading.value = true;
    testError.value = '';
    try {
      const result = await store.testTeam(testTeam.value.id, content);
      testReply.value = result.reply;
      testRun.value = result.run;
      $q.notify({ type: 'positive', message: 'Team 测试运行完成' });
    } catch (err) {
      testError.value = err instanceof Error ? err.message : i18n.global.t('teamsPage.runTestFailed');
    } finally {
      testLoading.value = false;
    }
  }

  // ── Navigation ──

  async function openTeamObservatory(team: Team) {
    try {
      const run = await store.findActiveRun(team.id);
      if (run) {
        router.push({ name: 'team-run-observatory', params: { teamId: team.id, runId: run.id } });
        return;
      }
      router.push({ name: 'team-orchestrate', params: { teamId: team.id } });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : i18n.global.t('teamsPage.openObservatoryFailed'),
      });
    }
  }

  function openRunObservatory(runID: string) {
    if (!store.selectedTeam) return;
    router.push({ name: 'team-run-observatory', params: { teamId: store.selectedTeam.id, runId: runID } });
  }

  // ── Retry (failed/cancelled → pending via backend state machine) ──

  async function retryTeam(team: Team) {
    try {
      await store.retryTeam(team.id);
      $q.notify({ type: 'positive', message: i18n.global.t('teamsPage.retrySuccess', { name: team.display_name }) });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : i18n.global.t('teamsPage.retryFailed'),
      });
    }
  }

  /** Retry from inside the editor dialog; syncs the readonly status display on success. */
  async function retryEditingTeam() {
    if (!editingId.value) return;
    try {
      const res = await store.retryTeam(editingId.value);
      form.status = res.status || 'pending';
      $q.notify({ type: 'positive', message: i18n.global.t('teamsPage.retrySuccessShort') });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : i18n.global.t('teamsPage.retryFailed'),
      });
    }
  }

  return {
    isDark,
    loading,
    saving,
    error,
    search,
    modeFilter,
    statusFilter,
    departmentFilter,
    showOrchestrated,
    hiddenOrchestratedCount,
    taxonomyTree,
    departmentOptions,
    teamGroups,
    currentPage,
    pageSize,
    totalFiltered,
    pageMax,
    editorOpen,
    selectedTeamTemplateKey,
    editingId,
    editingHasActiveRun,
    runsOpen,
    runsLoading,
    runsError,
    runEventsConnected,
    selectedTeam: computed(() => store.selectedTeam),
    runs,
    stepsByRun,
    stepsLoading,
    summariesByRun,
    summariesLoading,
    detailsByRun,
    detailsLoading,
    deadLetters,
    deadLettersLoading,
    testOpen,
    testTeam,
    testLoading,
    testError,
    testReply,
    testRun,
    form,
    definition,
    agentOptions,
    graphOptions,
    editorOverwriteBaselineKey,
    definitionJSON,
    canSave,
    filteredTeams,
    loadRows,
    openCreate,
    openEdit,
    resetToDerived,
    addMember,
    removeMember,
    applyTemplate,
    save,
    duplicate,
    confirmRemove,
    openRuns,
    openRunTest,
    executeRunTest,
    loadRunSummary,
    loadRunDetail,
    openRunObservatory,
    openTeamObservatory,
    loadRuns,
    loadRunSteps,
    loadDeadLetters,
    resolveDeadLetter,
    retryTeam,
    retryEditingTeam,
    storeAgents,
  };
}
