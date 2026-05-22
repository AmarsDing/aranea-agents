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
      <q-card class="graph-run-dialog app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">执行 Graph</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body q-gutter-md">
          <q-input v-model="runSessionId" class="app-field-md" dense outlined label="Session ID" />
          <q-input v-model="runInitialState" class="app-field-long" dense outlined autogrow type="textarea" label="初始状态 (JSON)" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded label="取消" @click="runDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="执行" :loading="runLoading" @click="executeRun" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import GraphNodePalette from "../components/graph/GraphNodePalette.vue";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphPropertyPanel from "../components/graph/GraphPropertyPanel.vue";
import { useGraphEditorPage } from "../features/graph/useGraphEditorPage";

const {
  isDark,
  isNew,
  saving,
  runDialogOpen,
  runSessionId,
  runInitialState,
  runLoading,
  availableTools,
  execNodeStates,
  graphDef,
  selectedNode,
  canSave,
  onSelectNode,
  markDirty,
  save,
  openRunDialog,
  executeRun,
  goBack
} = useGraphEditorPage();
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

.graph-editor-page.is-dark {
  background: var(--canvas-base, var(--canvas-base));
}

.graph-editor-page.is-dark .graph-editor-page__toolbar {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}
</style>
