import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import type { Agent } from "../agents/types";
import type { Team, TeamDefinition, TeamRun, TeamRunEvent, TeamRunStep, TeamRunSummary } from "./types";
import { findActiveTeamRun } from "./api";
import { useTeamsPageStore } from "../../stores/teams/page";
import { buildGraphFromDefinition, defaultDefinition, definitionFromTemplate, definitionToJSON, groupTeamsByIndustry, industryOptionsFromTree, parseDefinition, resetDefinition, type TeamTemplateKey } from "../../components/teams/teamUtils";
import { usePlatformStore } from "../../stores/platform";
import type { PlatformResourceTreeNode } from "../platform/types";

export function useTeamsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const teamsPageStore = useTeamsPageStore();
const isDark = computed(() => $q.dark.isActive);
const rows = ref<Team[]>([]);
const agents = ref<Agent[]>([]);
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const search = ref("");
const modeFilter = ref("");
const statusFilter = ref("");
const industryFilter = ref("");
const categoryTree = ref<PlatformResourceTreeNode[]>([]);
const editorOpen = ref(false);
const selectedTeamTemplateKey = ref<TeamTemplateKey | null>(null);
const editingId = ref("");
const runsOpen = ref(false);
const runsLoading = ref(false);
const runsError = ref("");
const runEventsConnected = ref(false);
const runEventsReplaying = ref(false);
const selectedTeam = ref<Team | null>(null);
const runs = ref<TeamRun[]>([]);
const stepsByRun = ref<Record<string, TeamRunStep[]>>({});
const stepsLoading = ref<Record<string, boolean>>({});
const summariesByRun = ref<Record<string, TeamRunSummary>>({});
const summariesLoading = ref<Record<string, boolean>>({});
const testOpen = ref(false);
const testTeam = ref<Team | null>(null);
const testLoading = ref(false);
const testError = ref("");
const testReply = ref("");
const testRun = ref<TeamRun | null>(null);
let runEventsSource: ReturnType<typeof teamsPageStore.subscribeRunEvents> | null = null;

const form = reactive({
  team_key: "",
  display_name: "",
  status: "active",
  app_name: ""
});

const definition = reactive<TeamDefinition>({
  version: 1,
  description: "",
  mode: "sequential",
  max_concurrency: 2,
  timeout_seconds: 600,
  loop_max_iterations: 0,
  members: []
});

const agentOptions = computed(() => agents.value.map((agent) => ({ label: agent.display_name, value: agent.id })));
const definitionJSON = computed(() => definitionToJSON(definition));
const canSave = computed(() => Boolean(form.team_key && form.display_name && definition.members.some((member) => member.enabled)));
const filteredTeams = computed(() => {
  const q = search.value.trim().toLowerCase();
  return rows.value.filter((team) => {
    const def = parseDefinition(team);
    const matchesSearch = !q || [team.display_name, team.team_key, def.description].some((value) => (value || "").toLowerCase().includes(q));
    const matchesMode = !modeFilter.value || def.mode === modeFilter.value;
    const matchesStatus = !statusFilter.value || team.status === statusFilter.value;
    return matchesSearch && matchesMode && matchesStatus;
  });
});
const industryOptions = computed(() => industryOptionsFromTree(categoryTree.value));
const teamIndustryGroups = computed(() =>
  groupTeamsByIndustry(filteredTeams.value, agents.value, categoryTree.value, industryFilter.value)
);

onMounted(loadRows);
onBeforeUnmount(closeRunEvents);
watch(
  () => route.query.edit,
  () => openRouteEdit()
);
watch(runsOpen, (open) => {
  if (!open) closeRunEvents();
});

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const platformStore = usePlatformStore();
    const [teamRows, agentRows] = await Promise.all([
      teamsPageStore.loadTeams(),
      teamsPageStore.loadAgents(),
      platformStore.loadCategoryTree()
    ]);
    rows.value = teamRows;
    agents.value = agentRows;
    categoryTree.value = platformStore.categoryTree;
    openRouteEdit();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Team 失败";
  } finally {
    loading.value = false;
  }
}

function openRouteEdit() {
  const editID = typeof route.query.edit === "string" ? route.query.edit : "";
  if (!editID || !rows.value.length) return;
  const team = rows.value.find((row) => row.id === editID);
  if (team) openEdit(team);
}

function openCreate() {
  editingId.value = "";
  selectedTeamTemplateKey.value = null;
  Object.assign(form, { team_key: "", display_name: "", status: "active", app_name: "" });
  resetDefinition(definition);
  editorOpen.value = true;
}

function openEdit(team: Team) {
  editingId.value = team.id;
  selectedTeamTemplateKey.value = null;
  Object.assign(form, { team_key: team.team_key, display_name: team.display_name, status: team.status, app_name: team.app_name });
  Object.assign(definition, parseDefinition(team));
  editorOpen.value = true;
}

function addMember() {
  definition.members.push({
    agent_id: agents.value[0]?.id ?? "",
    role: "worker",
    name: agents.value[0]?.display_name ?? "",
    enabled: true,
    sort_order: (definition.members.length + 1) * 10
  });
}

function removeMember(index: number) {
  definition.members.splice(index, 1);
}

function validateTeamDefinition(): string | null {
  const mode = String(definition.mode || "sequential").toLowerCase();
  const enabled = definition.members.filter((m) => m.enabled !== false && String(m.agent_id || "").trim() !== "");
  if (enabled.length === 0) {
    return "请至少启用一名成员并选择 Agent";
  }
  if (mode === "parallel") {
    const synthRaw = String(definition.synthesizer_agent_id || "").trim();
    const synthFromRole = enabled.find((m) => String(m.role || "").toLowerCase() === "synthesizer")?.agent_id?.trim();
    const synth = synthRaw || synthFromRole || "";
    if (!synth) {
      return "并行模式需要指定汇总 Agent（synthesizer_agent_id 或成员角色 synthesizer）";
    }
    const workers = enabled.filter((m) => String(m.agent_id).trim() !== synth);
    if (workers.length === 0) {
      return "并行模式至少需要一名与汇总 Agent 不同的并行成员";
    }
  }
  if (mode === "critic_loop" && enabled.length < 2) {
    return "生成评审模式建议至少两名成员（生成与评审）";
  }
  return null;
}

function applyTemplate(template: TeamTemplateKey) {
  if (agents.value.length === 0) {
    $q.notify({ type: "warning", message: "请先创建或加载 Agent 后再应用模板" });
    selectedTeamTemplateKey.value = null;
    return;
  }
  resetDefinition(definition);
  Object.assign(definition, definitionFromTemplate(template, agents.value));
  $q.notify({ type: "positive", message: "Team 模板已应用" });
}

async function save() {
  const hint = validateTeamDefinition();
  if (hint) {
    $q.notify({ type: "warning", message: hint });
    return;
  }
  saving.value = true;
  try {
    const payload = {
      team_key: form.team_key,
      display_name: form.display_name,
      status: form.status,
      app_name: form.app_name || form.team_key,
      definition_json: definitionJSON.value
    };
    const saved = editingId.value ? await teamsPageStore.editTeam(editingId.value, payload) : await teamsPageStore.addTeam(payload);
    rows.value = editingId.value ? rows.value.map((row) => (row.id === saved.id ? saved : row)) : [saved, ...rows.value];
    editorOpen.value = false;
    $q.notify({ type: "positive", message: "Team 已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}

async function duplicate(team: Team) {
  try {
    const created = await teamsPageStore.copyTeam(team.id);
    rows.value = [created, ...rows.value];
    $q.notify({ type: "positive", message: "Team 已复制" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "复制失败" });
  }
}

function confirmRemove(team: Team) {
  $q.dialog({
    title: "删除 Team",
    message: `确定删除「${team.display_name}」？此操作不可撤销。`,
    cancel: true,
    persistent: true,
  }).onOk(() => void remove(team));
}

async function remove(team: Team) {
  try {
    await teamsPageStore.removeTeam(team.id);
    rows.value = rows.value.filter((row) => row.id !== team.id);
    $q.notify({ type: "info", message: "Team 已删除" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
  }
}

async function copyKey(value: string) {
  await copyToClipboard(value);
  $q.notify({ type: "positive", message: "Team Key 已复制" });
}

async function openRuns(team: Team) {
  selectedTeam.value = team;
  runsOpen.value = true;
  summariesByRun.value = {};
  stepsByRun.value = {};
  await loadRuns();
  openRunEvents(team.id);
}

function openRunTest(team: Team) {
  testTeam.value = team;
  testError.value = "";
  testReply.value = "";
  testRun.value = null;
  testOpen.value = true;
}

async function executeRunTest(content: string) {
  if (!testTeam.value) return;
  testLoading.value = true;
  testError.value = "";
  try {
    const result = await teamsPageStore.testTeam(testTeam.value.id, content);
    testReply.value = result.reply;
    testRun.value = result.run;
    $q.notify({ type: "positive", message: "Team 测试运行完成" });
  } catch (err) {
    testError.value = err instanceof Error ? err.message : "运行测试失败";
  } finally {
    testLoading.value = false;
  }
}

async function loadRunSummary(runID: string) {
  if (summariesByRun.value[runID] || summariesLoading.value[runID]) return;
  summariesLoading.value = { ...summariesLoading.value, [runID]: true };
  try {
    const summary = await teamsPageStore.loadRunSummary(runID);
    summariesByRun.value = { ...summariesByRun.value, [runID]: summary };
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载汇总失败" });
  } finally {
    summariesLoading.value = { ...summariesLoading.value, [runID]: false };
  }
}

function openRunObservatory(runID: string) {
  if (!selectedTeam.value) return;
  router.push({
    name: "team-run-observatory",
    params: { teamId: selectedTeam.value.id, runId: runID },
  });
}

async function openTeamObservatory(team: Team) {
  try {
    const run = await findActiveTeamRun(team.id);
    if (run) {
      router.push({
        name: "team-run-observatory",
        params: { teamId: team.id, runId: run.id },
      });
      return;
    }
    router.push({ name: "team-orchestrate", params: { teamId: team.id } });
  } catch (err) {
    $q.notify({
      type: "negative",
      message: err instanceof Error ? err.message : "打开观测台失败",
    });
  }
}

async function loadRuns() {
  if (!selectedTeam.value) return;
  runsLoading.value = true;
  runsError.value = "";
  try {
    runs.value = await teamsPageStore.loadRuns(selectedTeam.value.id, 30);
  } catch (err) {
    runsError.value = err instanceof Error ? err.message : "加载运行轨迹失败";
  } finally {
    runsLoading.value = false;
  }
}

async function loadRunSteps(runID: string) {
  if (stepsByRun.value[runID]?.length || stepsLoading.value[runID]) return;
  stepsLoading.value = { ...stepsLoading.value, [runID]: true };
  try {
    const steps = await teamsPageStore.loadRunSteps(runID);
    stepsByRun.value = { ...stepsByRun.value, [runID]: steps };
  } finally {
    stepsLoading.value = { ...stepsLoading.value, [runID]: false };
  }
}

function openRunEvents(teamID: string) {
  closeRunEvents();
  runEventsSource = teamsPageStore.subscribeRunEvents(
    GLOBAL_WS_SESSION_ID,
    teamID,
    (event) => {
      runEventsConnected.value = true;
      applyRunEvent(event);
    },
    () => {
      runEventsConnected.value = false;
    },
    (replaying) => {
      runEventsReplaying.value = replaying;
    }
  );
}

function closeRunEvents() {
  runEventsSource?.close();
  runEventsSource = null;
  runEventsConnected.value = false;
  runEventsReplaying.value = false;
}

function applyRunEvent(event: TeamRunEvent) {
  if (selectedTeam.value && event.team_id !== selectedTeam.value.id) return;
  if (event.run) {
    upsertRun(event.run);
  }
  if (event.step) {
    upsertRunStep(event.step);
  }
}

function upsertRun(run: TeamRun) {
  const index = runs.value.findIndex((item) => item.id === run.id);
  if (index >= 0) {
    runs.value = runs.value.map((item) => (item.id === run.id ? run : item));
    return;
  }
  runs.value = [run, ...runs.value].slice(0, 30);
}

function upsertRunStep(step: TeamRunStep) {
  const current = stepsByRun.value[step.run_id] ?? [];
  const exists = current.some((item) => item.id === step.id);
  const next = exists ? current.map((item) => (item.id === step.id ? step : item)) : [...current, step];
  next.sort((a, b) => a.sort_order - b.sort_order || a.created_at.localeCompare(b.created_at));
  stepsByRun.value = { ...stepsByRun.value, [step.run_id]: next };
}

  return {
    isDark, rows, agents, loading, saving, error, search, modeFilter, statusFilter, industryFilter,
    categoryTree, industryOptions, teamIndustryGroups,
    editorOpen, selectedTeamTemplateKey, editingId, runsOpen, runsLoading, runsError,
    runEventsConnected, runEventsReplaying, selectedTeam, runs, stepsByRun, stepsLoading,
    summariesByRun, summariesLoading, testOpen, testTeam, testLoading, testError, testReply, testRun,
    form, definition, agentOptions, definitionJSON, canSave, filteredTeams,
    loadRows, openCreate, openEdit, addMember, removeMember, applyTemplate, save, duplicate, confirmRemove,
    copyKey, openRuns, openRunTest, executeRunTest, loadRunSummary, openRunObservatory, openTeamObservatory, loadRuns, loadRunSteps
  };
}
