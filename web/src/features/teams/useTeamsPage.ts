import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { copyToClipboard, useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { Agent } from '../agents/types';
import type { Team, TeamDefinition, TeamRun, TeamRunEvent, TaskDeadLetterRow } from './types';
import { listTaskDeadLetters, resolveTaskDeadLetter } from './api';
import { useTeamsStore } from '../../stores/teams';
import { usePlatformStore } from '../../stores/platform';
import {
  definitionFromTemplate,
  definitionToJSON,
  groupTeamsByIndustry,
  industryOptionsFromTree,
  parseDefinition,
  resetDefinition,
  validateTeamDefinition,
  type TeamTemplateKey,
} from '../../components/teams/teamUtils';
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
  } = storeToRefs(store);

  // ── List state ──
  const loading = ref(false);
  const error = ref('');
  const search = ref('');
  const modeFilter = ref('');
  const statusFilter = ref('');
  const industryFilter = ref('');
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

  // ── Runs UI state ──
  const runsOpen = ref(false);
  const runEventsConnected = ref(false);
  let runEventsSource: ReturnType<typeof store.subscribeRunEvents> | null = null;
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
  const definitionJSON = computed(() => definitionToJSON(definition));
  const canSave = computed(() =>
    Boolean(form.team_key && form.display_name && definition.members.some((member) => member.enabled)),
  );
  const filteredTeams = computed(() => {
    const q = search.value.trim().toLowerCase();
    return store.teams.filter((team) => {
      const def = parseDefinition(team);
      const matchesSearch =
        !q ||
        [team.display_name, team.team_key, def.description].some((value) => (value || '').toLowerCase().includes(q));
      const matchesMode = !modeFilter.value || def.mode === modeFilter.value;
      const matchesStatus = !statusFilter.value || team.status === statusFilter.value;
      return matchesSearch && matchesMode && matchesStatus;
    });
  });
  const industryOptions = computed(() => industryOptionsFromTree(taxonomyTree.value));
  const totalFiltered = computed(() => filteredTeams.value.length);
  const pageMax = computed(() => Math.max(1, Math.ceil(totalFiltered.value / pageSize.value)));
  const paginatedTeams = computed(() => {
    const start = (currentPage.value - 1) * pageSize.value;
    return filteredTeams.value.slice(start, start + pageSize.value);
  });
  const teamIndustryGroups = computed(() =>
    groupTeamsByIndustry(paginatedTeams.value, store.agents, taxonomyTree.value, industryFilter.value),
  );

  watch([search, modeFilter, statusFilter, industryFilter], () => {
    currentPage.value = 1;
  });

  onMounted(loadRows);
  onBeforeUnmount(closeRunEvents);
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

  function openCreate() {
    editingId.value = '';
    editingHasActiveRun.value = false;
    selectedTeamTemplateKey.value = null;
    Object.assign(form, { team_key: '', display_name: '', status: 'pending', app_name: '', taxonomy_industry_id: '' });
    resetDefinition(definition);
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
    });
    Object.assign(definition, parseDefinition(team));
    editorOpen.value = true;
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
    resetDefinition(definition);
    Object.assign(definition, definitionFromTemplate(template, store.agents));
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

  async function copyKey(value: string) {
    await copyToClipboard(value);
    $q.notify({ type: 'positive', message: 'Team Key 已复制' });
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

  function openRunEvents(teamID: string) {
    closeRunEvents();
    runEventsSource = store.subscribeRunEvents(GLOBAL_WS_SESSION_ID, teamID, (event) => {
      runEventsConnected.value = true;
      applyRunEvent(event);
    });
  }

  function closeRunEvents() {
    runEventsSource?.close();
    runEventsSource = null;
    runEventsConnected.value = false;
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
      testError.value = err instanceof Error ? err.message : '运行测试失败';
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
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '打开观测台失败' });
    }
  }

  function openRunObservatory(runID: string) {
    if (!store.selectedTeam) return;
    router.push({ name: 'team-run-observatory', params: { teamId: store.selectedTeam.id, runId: runID } });
  }

  // ── Reorder (local-only until backend API) ──
  // TECH-DEBT: reorder is local-only; add backend persistence API — issue #TBD
  function reorderTeams(ids: string[]) {
    const idIndex = new Map(ids.map((id, i) => [id, i]));
    const reordered = [...store.teams].sort((a, b) => {
      const ai = idIndex.get(a.id);
      const bi = idIndex.get(b.id);
      if (ai !== undefined && bi !== undefined) return ai - bi;
      if (ai !== undefined) return -1;
      if (bi !== undefined) return 1;
      return 0;
    });
    store.teams.splice(0, store.teams.length, ...reordered);
  }

  return {
    isDark,
    loading,
    saving,
    error,
    search,
    modeFilter,
    statusFilter,
    industryFilter,
    taxonomyTree,
    industryOptions,
    teamIndustryGroups,
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
    definitionJSON,
    canSave,
    filteredTeams,
    loadRows,
    openCreate,
    openEdit,
    addMember,
    removeMember,
    applyTemplate,
    save,
    duplicate,
    confirmRemove,
    copyKey,
    openRuns,
    openRunTest,
    executeRunTest,
    loadRunSummary,
    openRunObservatory,
    openTeamObservatory,
    loadRuns,
    loadRunSteps,
    loadDeadLetters,
    resolveDeadLetter,
    reorderTeams,
    storeAgents,
  };
}
