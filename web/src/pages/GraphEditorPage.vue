<template>
  <q-page :class="['graph-editor-page', { 'is-dark': isDark }]">
    <div class="graph-editor-page__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-editor-page__title">{{ isNew ? '新增 Graph' : graphDef.name || '编辑 Graph' }}</div>
      <q-space />
      <q-btn flat dense round icon="save" color="primary" :loading="saving" :disable="!canSave" @click="save">
        <q-tooltip>保存</q-tooltip>
      </q-btn>
      <q-btn v-if="!isNew" flat dense round icon="play_arrow" color="positive" @click="openRunDialog">
        <q-tooltip>执行</q-tooltip>
      </q-btn>
    </div>

    <div class="graph-editor-page__body">
      <GraphNodePalette :is-dark="isDark" />
      <GraphEditorCanvas
        :graph-def="graphDef"
        :is-dark="isDark"
        :exec-node-states="execNodeStates"
        @select-node="onSelectNode"
        @update-graph="markDirty"
      />
      <GraphPropertyPanel
        :selected-node="selectedNode"
        :graph-def="graphDef"
        :available-tools="availableTools"
        :is-dark="isDark"
        @deselect="onSelectNode(null)"
      />
    </div>

    <q-dialog v-model="runDialogOpen" persistent>
      <q-card :class="['graph-run-dialog', { 'is-dark': isDark }]">
        <q-card-section>
          <div class="text-h6">执行 Graph</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <q-input v-model="runSessionId" dense outlined label="Session ID" />
          <q-input v-model="runInitialState" dense outlined autogrow type="textarea" label="初始状态 (JSON)" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" @click="runDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="执行" :loading="runLoading" @click="executeRun" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import {
  getGraph,
  createGraph,
  updateGraph,
  executeGraph,
  type GraphDefinition,
  type NodeDef,
} from "../features/graph/api";
import GraphNodePalette from "../components/graph/GraphNodePalette.vue";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphPropertyPanel from "../components/graph/GraphPropertyPanel.vue";

const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const isDark = computed(() => $q.dark.isActive);

const isNew = computed(() => route.name === "graph-editor-new");
const graphId = computed(() => (route.params.id as string) ?? "");

const saving = ref(false);
const dirty = ref(false);
const runDialogOpen = ref(false);
const runSessionId = ref("");
const runInitialState = ref("");
const runLoading = ref(false);
const selectedNodeId = ref<string | null>(null);
const availableTools = ref<string[]>([]);
const execNodeStates = ref<Map<string, { status: string }>>(new Map());

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
  enableCheckpoint: true,
  executionEngine: "bsp",
  interruptBefore: [],
  interruptAfter: [],
  metadata: {},
  createdAt: "",
  updatedAt: "",
});

const selectedNode = computed<NodeDef | null>(() => {
  if (!selectedNodeId.value) return null;
  return graphDef.nodes.find((n) => n.id === selectedNodeId.value) ?? null;
});

const canSave = computed(() => Boolean(graphDef.name && graphDef.nodes.length > 0));

onMounted(async () => {
  if (!isNew.value && graphId.value) {
    try {
      const g = await getGraph(graphId.value);
      Object.assign(graphDef, g);
    } catch (err) {
      $q.notify({ type: "negative", message: "加载 Graph 失败" });
    }
  }
});

function onSelectNode(nodeId: string | null) {
  selectedNodeId.value = nodeId;
}

function markDirty() {
  dirty.value = true;
}

async function save() {
  saving.value = true;
  try {
    if (isNew.value || !graphDef.id) {
      const created = await createGraph(graphDef);
      Object.assign(graphDef, created);
      dirty.value = false;
      $q.notify({ type: "positive", message: "Graph 已创建" });
      router.replace({ name: "graph-editor", params: { id: created.id } });
    } else {
      const updated = await updateGraph(graphDef.id, graphDef);
      Object.assign(graphDef, updated);
      dirty.value = false;
      $q.notify({ type: "positive", message: "Graph 已保存" });
    }
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}

function openRunDialog() {
  runSessionId.value = `graph-${Date.now()}`;
  runInitialState.value = "";
  runDialogOpen.value = true;
}

async function executeRun() {
  runLoading.value = true;
  try {
    let initialState: Record<string, unknown> | undefined;
    if (runInitialState.value.trim()) {
      initialState = JSON.parse(runInitialState.value);
    }
    const result = await executeGraph(graphDef.id, runSessionId.value, initialState);
    runDialogOpen.value = false;
    $q.notify({ type: "positive", message: `Graph 执行已启动：${result.executionId}` });
    router.push({
      name: "graph-run",
      params: { id: graphDef.id, execId: result.executionId },
    });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "执行失败" });
  } finally {
    runLoading.value = false;
  }
}

function goBack() {
  router.push({ name: "graphs" });
}

onBeforeRouteLeave((_to, _from, next) => {
  if (dirty.value) {
    $q.dialog({
      title: "未保存的更改",
      message: "当前 Graph 有未保存的更改，确定要离开吗？",
      cancel: true,
      persistent: true,
    }).onOk(() => {
      next();
    }).onCancel(() => {
      next(false);
    });
  } else {
    next();
  }
});
</script>

<style scoped>
.graph-editor-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--canvas-base, var(--canvas-base));
}

.graph-editor-page__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  background: var(--glass-surface, rgb(255 253 245 / 65%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
}

.graph-editor-page__title {
  font-size: 15px;
  font-weight: 700;
}

.graph-editor-page__body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.graph-run-dialog {
  width: 480px;
  max-width: 94vw;
  border: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  border-radius: 24px;
  background: var(--glass-elevated, rgb(255 255 255 / 72%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
}

.graph-editor-page.is-dark {
  background: var(--canvas-base, var(--canvas-base));
}

.graph-editor-page.is-dark .graph-editor-page__toolbar {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}

.graph-run-dialog.is-dark {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 90%);
}
</style>
