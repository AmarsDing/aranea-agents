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
      <q-card class="graph-resume-dialog app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">恢复执行</div>
          <div class="text-caption text-grey-7">输入恢复值后继续执行</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body">
          <q-input v-model="resumeValue" class="app-field-long" dense outlined autogrow type="textarea" label="恢复值 (JSON)" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded label="取消" @click="resumeDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="恢复" :loading="resumeLoading" @click="doResume" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import { useGraphRunPage } from "../features/graph/useGraphRunPage";

const {
  isDark,
  graphDef,
  execution,
  execNodeStates,
  resumeDialogOpen,
  resumeValue,
  resumeLoading,
  statusColor,
  onSelectNode,
  cancelExec,
  resumeExec,
  doResume,
  stepIcon,
  stepColor,
  formatTime,
  goBack
} = useGraphRunPage();
</script>

<style scoped>
.graph-run-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--canvas-base, var(--canvas-base));
}

.graph-run-page__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  background: var(--glass-surface, rgb(255 253 245 / 65%));
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
  border-left: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  background: var(--glass-surface, rgb(255 253 245 / 65%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
  overflow-y: auto;
}

.graph-run-page__sidebar-title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, var(--color-text-secondary));
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
  border: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  border-radius: 24px;
  background: var(--glass-elevated, rgb(255 255 255 / 72%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
}

.graph-run-page.is-dark {
  background: var(--canvas-base, var(--canvas-base));
}

.graph-run-page.is-dark .graph-run-page__toolbar {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}

.graph-run-page__sidebar.is-dark {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}

.graph-resume-dialog.is-dark {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 90%);
}
</style>
