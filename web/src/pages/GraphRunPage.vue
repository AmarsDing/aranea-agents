<template>
  <q-page :class="['graph-run-page', { 'is-dark': isDark }]">
    <div class="graph-run-page__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-run-page__title">执行监控</div>
      <q-space />
      <q-badge rounded :color="statusColor">{{ execution?.status ?? "loading" }}</q-badge>
      <q-btn v-if="execution?.status === 'running'" flat dense round icon="stop" color="negative" @click="cancelExec">
        <q-tooltip>取消执行</q-tooltip>
      </q-btn>
      <q-btn v-if="execution?.status === 'waiting_human'" flat dense round icon="play_arrow" color="positive" @click="resumeExec">
        <q-tooltip>恢复执行</q-tooltip>
      </q-btn>
    </div>

    <div class="graph-run-page__body">
      <GraphEditorCanvas
        :graph-def="graphDef"
        :is-dark="isDark"
        :exec-node-states="execNodeStates"
        @select-node="onSelectNode"
        @update-graph="() => {}"
      />
      <div :class="['graph-run-page__sidebar', { 'is-dark': isDark }]">
        <div class="graph-run-page__sidebar-title">执行详情</div>
        <template v-if="execution">
          <div class="q-gutter-sm">
            <div class="text-caption text-grey-7">执行 ID</div>
            <div class="text-body2 text-mono">{{ execution.executionId }}</div>
            <q-separator />
            <div class="text-caption text-grey-7">状态</div>
            <q-badge rounded :color="statusColor">{{ execution.status }}</q-badge>
            <div v-if="execution.startedAt" class="text-caption text-grey-7 q-mt-sm">开始时间</div>
            <div v-if="execution.startedAt" class="text-body2">{{ formatTime(execution.startedAt) }}</div>
            <div v-if="execution.finishedAt" class="text-caption text-grey-7 q-mt-sm">结束时间</div>
            <div v-if="execution.finishedAt" class="text-body2">{{ formatTime(execution.finishedAt) }}</div>
          </div>
          <q-separator class="q-my-md" />
          <div class="graph-run-page__sidebar-title">步骤时间线</div>
          <q-timeline color="primary" dense>
            <q-timeline-entry
              v-for="step in execution.steps"
              :key="step.stepIndex"
              :title="step.nodeId"
              :subtitle="formatTime(step.timestamp)"
              :icon="stepIcon(step.status)"
              :color="stepColor(step.status)"
            >
              <div class="text-caption">{{ step.status }}</div>
              <div v-if="step.error" class="text-negative text-caption">{{ step.error }}</div>
            </q-timeline-entry>
          </q-timeline>
        </template>
        <q-spinner v-else color="primary" size="32px" />
      </div>
    </div>

    <q-dialog v-model="resumeDialogOpen" persistent>
      <q-card :class="['graph-resume-dialog', { 'is-dark': isDark }]">
        <q-card-section>
          <div class="text-h6">恢复执行</div>
          <div class="text-caption text-grey-7">输入恢复值后继续执行</div>
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-input v-model="resumeValue" dense outlined autogrow type="textarea" label="恢复值 (JSON)" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" @click="resumeDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="恢复" :loading="resumeLoading" @click="doResume" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import {
  getGraph,
  getGraphExecution,
  cancelGraphExecution,
  resumeGraph,
  type GraphDefinition,
  type GraphExecution,
} from "../features/graph/api";
import { useGraphStream } from "../features/chat/useEnvelopeStream";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";

const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const isDark = computed(() => $q.dark.isActive);

const graphId = computed(() => (route.params.id as string) ?? "");
const execId = computed(() => (route.params.execId as string) ?? "");

const graphDef = reactive<GraphDefinition>({
  id: "",
  name: "",
  description: "",
  stateFields: [],
  nodes: [],
  edges: [],
  conditionalEdges: [],
  subgraphs: [],
  entryPoint: "",
  finishPoint: "",
  enableCheckpoint: false,
  executionEngine: "bsp",
  interruptBefore: [],
  interruptAfter: [],
  metadata: {},
  createdAt: "",
  updatedAt: "",
});

const execution = ref<GraphExecution | null>(null);
const execNodeStates = ref<Map<string, { status: string }>>(new Map());
const resumeDialogOpen = ref(false);
const resumeValue = ref("");
const resumeLoading = ref(false);

let graphStream: ReturnType<typeof useGraphStream> | null = null;

const statusColor = computed(() => {
  const s = execution.value?.status ?? "";
  if (s === "completed") return "positive";
  if (s === "running") return "blue";
  if (s === "failed") return "negative";
  if (s === "waiting_human") return "warning";
  return "grey";
});

onMounted(async () => {
  if (graphId.value) {
    try {
      Object.assign(graphDef, await getGraph(graphId.value));
    } catch {
      $q.notify({ type: "negative", message: "加载 Graph 失败" });
    }
  }
  if (execId.value) {
    try {
      execution.value = await getGraphExecution(execId.value);
      syncNodeStates();
    } catch {
      $q.notify({ type: "negative", message: "加载执行信息失败" });
    }
  }
  if (graphId.value && execId.value) {
    graphStream = useGraphStream("graph-monitor", graphId.value, execId.value);
  }
});

onBeforeUnmount(() => {
  graphStream?.disconnect();
});

function syncNodeStates() {
  const map = new Map<string, { status: string }>();
  if (execution.value) {
    for (const step of execution.value.steps) {
      map.set(step.nodeId, { status: step.status });
    }
  }
  execNodeStates.value = map;
}

function onSelectNode(_nodeId: string | null) {}

async function cancelExec() {
  if (!execId.value) return;
  try {
    await cancelGraphExecution(execId.value);
    $q.notify({ type: "info", message: "执行已取消" });
    execution.value = await getGraphExecution(execId.value);
    syncNodeStates();
  } catch (err) {
    $q.notify({ type: "negative", message: "取消失败" });
  }
}

function resumeExec() {
  resumeValue.value = "";
  resumeDialogOpen.value = true;
}

async function doResume() {
  if (!execId.value) return;
  resumeLoading.value = true;
  try {
    let value: Record<string, unknown> | undefined;
    if (resumeValue.value.trim()) {
      value = JSON.parse(resumeValue.value);
    }
    await resumeGraph(execId.value, value);
    resumeDialogOpen.value = false;
    $q.notify({ type: "positive", message: "执行已恢复" });
    execution.value = await getGraphExecution(execId.value);
    syncNodeStates();
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "恢复失败" });
  } finally {
    resumeLoading.value = false;
  }
}

function stepIcon(status: string) {
  if (status === "completed") return "check_circle";
  if (status === "error" || status === "failed") return "error";
  if (status === "running") return "sync";
  return "radio_button_unchecked";
}

function stepColor(status: string) {
  if (status === "completed") return "positive";
  if (status === "error" || status === "failed") return "negative";
  if (status === "running") return "blue";
  return "grey";
}

function formatTime(ts: string) {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}

function goBack() {
  router.push({ name: "graphs" });
}
</script>

<style scoped>
.graph-run-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--canvas-base, #fefbf4);
}

.graph-run-page__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7));
  background: var(--glass-surface, rgba(255, 253, 245, 0.65));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
}

.graph-run-page__title {
  font-size: 15px;
  font-weight: 700;
}

.graph-run-page__body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.graph-run-page__sidebar {
  width: 300px;
  padding: 16px;
  border-left: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7));
  background: var(--glass-surface, rgba(255, 253, 245, 0.65));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
  overflow-y: auto;
}

.graph-run-page__sidebar-title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, #8b7a6b);
  margin-bottom: 10px;
}

.text-mono {
  font-family: monospace;
  font-size: 11px;
  word-break: break-all;
}

.graph-resume-dialog {
  width: 480px;
  max-width: 94vw;
  border: 1px solid var(--glass-border, rgba(235, 220, 200, 0.7));
  border-radius: 24px;
  background: var(--glass-elevated, rgba(255, 255, 255, 0.72));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
}

.graph-run-page.is-dark {
  background: var(--canvas-base, #090d14);
}

.graph-run-page.is-dark .graph-run-page__toolbar {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(18, 24, 34, 0.65);
}

.graph-run-page__sidebar.is-dark {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(18, 24, 34, 0.65);
}

.graph-resume-dialog.is-dark {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(18, 24, 34, 0.9);
}
</style>
