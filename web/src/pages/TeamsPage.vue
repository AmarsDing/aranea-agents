<template>
  <q-page :class="['teams-page', { 'is-dark': isDark }]">
    <section class="teams-hero">
      <div>
        <div class="teams-kicker">ADK Multi-Agent</div>
        <h1 class="teams-title">Team 管理</h1>
        <p class="teams-subtitle">参照 ADK Web 的 App / Session / Trace 工作台，将多个 Agent 编排成可运行、可观测的协作团队。</p>
      </div>
      <q-btn color="primary" rounded unelevated icon="add" label="新增 Team" @click="openCreate" />
    </section>

    <TeamToolbar
      v-model:search="search"
      v-model:mode-filter="modeFilter"
      v-model:status-filter="statusFilter"
      class="q-mt-md"
      :loading="loading"
      :is-dark="isDark"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" label="重试" @click="loadRows" /></template>
    </q-banner>

    <section class="teams-grid q-mt-lg">
      <TeamCard
        v-for="team in filteredTeams"
        :key="team.id"
        :team="team"
        :agents="agents"
        :is-dark="isDark"
        @copy-key="copyKey"
        @open-runs="openRuns"
        @duplicate="duplicate"
        @edit="openEdit"
        @remove="remove"
      />
    </section>

    <q-card v-if="!loading && filteredTeams.length === 0" flat bordered :class="['teams-empty', { 'is-dark': isDark }, 'q-mt-lg']">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="hub" />
        <div class="text-h6 q-mt-md">暂无 Team</div>
        <div class="text-body2 text-grey-7 q-mt-sm">创建一个 Team，把多个 Agent 组织成顺序、并行或评审闭环。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增 Team" @click="openCreate" />
      </q-card-section>
    </q-card>

    <TeamEditorDialog
      v-model="editorOpen"
      v-model:selected-template-key="selectedTeamTemplateKey"
      :editing-id="editingId"
      :form="form"
      :definition="definition"
      :definition-json="definitionJSON"
      :agent-options="agentOptions"
      :saving="saving"
      :can-save="canSave"
      :is-dark="isDark"
      @add-member="addMember"
      @remove-member="removeMember"
      @apply-template="applyTemplate"
      @save="save"
    />

    <TeamRunsDialog
      v-model="runsOpen"
      :selected-team="selectedTeam"
      :runs="runs"
      :steps-by-run="stepsByRun"
      :steps-loading="stepsLoading"
      :agents="agents"
      :loading="runsLoading"
      :error="runsError"
      :live-connected="runEventsConnected"
      :is-dark="isDark"
      @refresh="loadRuns"
      @show-steps="loadRunSteps"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute } from "vue-router";
import { GLOBAL_WS_SESSION_ID } from "../config/runtime";
import { listAgents, type Agent } from "../features/agents/api";
import {
  createTeam,
  deleteTeam,
  duplicateTeam,
  listTeamRuns,
  listTeamRunSteps,
  listTeams,
  subscribeTeamRunEventsWs,
  updateTeam,
  type Team,
  type TeamDefinition,
  type TeamRun,
  type TeamRunEvent,
  type TeamRunStep
} from "../features/teams/api";
import TeamCard from "../components/teams/TeamCard.vue";
import TeamEditorDialog from "../components/teams/TeamEditorDialog.vue";
import TeamRunsDialog from "../components/teams/TeamRunsDialog.vue";
import TeamToolbar from "../components/teams/TeamToolbar.vue";
import { buildGraphFromDefinition, defaultDefinition, definitionFromTemplate, parseDefinition, type TeamTemplateKey } from "../components/teams/teamUtils";

const $q = useQuasar();
const route = useRoute();
const isDark = computed(() => $q.dark.isActive);
const rows = ref<Team[]>([]);
const agents = ref<Agent[]>([]);
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const search = ref("");
const modeFilter = ref("");
const statusFilter = ref("");
const editorOpen = ref(false);
const selectedTeamTemplateKey = ref<TeamTemplateKey | null>(null);
const editingId = ref("");
const runsOpen = ref(false);
const runsLoading = ref(false);
const runsError = ref("");
const runEventsConnected = ref(false);
const selectedTeam = ref<Team | null>(null);
const runs = ref<TeamRun[]>([]);
const stepsByRun = ref<Record<string, TeamRunStep[]>>({});
const stepsLoading = ref<Record<string, boolean>>({});
let runEventsSource: ReturnType<typeof subscribeTeamRunEventsWs> | null = null;

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
const definitionJSON = computed(() => JSON.stringify({ ...definition, graph: buildGraphFromDefinition(definition) }, null, 2));
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
    const [teamRows, agentRows] = await Promise.all([listTeams(), listAgents({ limit: 100 })]);
    rows.value = teamRows;
    agents.value = agentRows;
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
  Object.assign(definition, defaultDefinition());
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
    const saved = editingId.value ? await updateTeam(editingId.value, payload) : await createTeam(payload);
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
  const created = await duplicateTeam(team.id);
  rows.value = [created, ...rows.value];
  $q.notify({ type: "positive", message: "Team 已复制" });
}

async function remove(team: Team) {
  await deleteTeam(team.id);
  rows.value = rows.value.filter((row) => row.id !== team.id);
  $q.notify({ type: "info", message: "Team 已删除" });
}

async function copyKey(value: string) {
  await copyToClipboard(value);
  $q.notify({ type: "positive", message: "Team Key 已复制" });
}

async function openRuns(team: Team) {
  selectedTeam.value = team;
  runsOpen.value = true;
  await loadRuns();
  openRunEvents(team.id);
}

async function loadRuns() {
  if (!selectedTeam.value) return;
  runsLoading.value = true;
  runsError.value = "";
  try {
    runs.value = await listTeamRuns(selectedTeam.value.id, 30);
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
    const steps = await listTeamRunSteps(runID);
    stepsByRun.value = { ...stepsByRun.value, [runID]: steps };
  } finally {
    stepsLoading.value = { ...stepsLoading.value, [runID]: false };
  }
}

function openRunEvents(teamID: string) {
  closeRunEvents();
  runEventsSource = subscribeTeamRunEventsWs(
    GLOBAL_WS_SESSION_ID,
    teamID,
    (event) => {
      runEventsConnected.value = true;
      applyRunEvent(event);
    },
    () => {
      runEventsConnected.value = false;
    }
  );
}

function closeRunEvents() {
  runEventsSource?.close();
  runEventsSource = null;
  runEventsConnected.value = false;
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

</script>

<style scoped>
.teams-page {
  min-height: 100%;
  padding: 28px;
  background:
    radial-gradient(circle at 86% 0%, rgb(25 118 210 / 12%), transparent 28%),
    linear-gradient(180deg, var(--color-page-tint) 0%, var(--color-page-tint-alt) 46%, var(--color-on-accent) 100%);
}

.teams-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.teams-kicker {
  display: inline-flex;
  padding: 5px 11px;
  border: 1px solid rgb(25 118 210 / 14%);
  border-radius: 999px;
  background: rgb(255 255 255 / 78%);
  color: var(--color-link);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.teams-title {
  margin: 12px 0 0;
  color: var(--color-text-dark);
  font-size: clamp(34px, 5vw, 54px);
  font-weight: 800;
  letter-spacing: -0.055em;
  line-height: 1;
}

.teams-subtitle {
  max-width: 720px;
  margin: 10px 0 0;
  color: var(--color-text-tertiary);
  line-height: 1.65;
}

.teams-empty {
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background: rgb(255 255 255 / 86%);
  box-shadow: 0 18px 48px rgb(16 24 40 / 6%);
  backdrop-filter: blur(16px);
}

.teams-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 18px;
}

.teams-page.is-dark {
  background:
    radial-gradient(circle at 86% 0%, rgb(59 130 246 / 16%), transparent 30%),
    linear-gradient(160deg, var(--canvas-base) 0%, var(--color-surface-elevated) 48%, var(--color-surface-solid) 100%);
  color: var(--color-border-soft);
}

.teams-page.is-dark .teams-kicker {
  border-color: rgb(96 165 250 / 22%);
  background: rgb(30 64 175 / 24%);
  color: var(--color-link);
}

.teams-page.is-dark .teams-title {
  color: var(--color-surface-soft);
}

.teams-page.is-dark .teams-subtitle {
  color: var(--color-text-tertiary);
}

.teams-empty.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 90%);
  box-shadow: 0 14px 38px rgb(0 0 0 / 32%);
}

@media (width <= 599px) {
  .teams-page {
    padding: 18px;
  }

  .teams-grid {
    grid-template-columns: 1fr;
  }
}
</style>
