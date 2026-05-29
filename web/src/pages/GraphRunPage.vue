<template>
  <q-page :class="['graph-workbench graph-run-page', { 'is-dark': isDark }]">
    <div class="graph-run-page__toolbar graph-workbench__toolbar">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="graph-workbench__toolbar-meta">
        <div class="graph-run-page__title">{{ graphDef.name || "执行监控" }}</div>
        <div class="graph-workbench__subtitle">
          {{ displayStatus }}
          <span v-if="streamConnected"> · 实时</span>
        </div>
      </div>
      <q-space />
      <q-badge v-if="streamConnected" rounded class="graph-run-page__live-badge">实时</q-badge>
      <q-badge rounded :color="statusColor">{{ displayStatus }}</q-badge>
      <span v-if="progressStepLabel && showProgressBar" class="graph-run-page__step-counter">{{ progressStepLabel }}</span>
      <q-btn v-if="displayStatus === 'running'" flat dense round icon="stop" color="negative" @click="confirmCancelExec">
        <q-tooltip>取消执行</q-tooltip>
      </q-btn>
      <q-btn v-if="displayStatus === 'waiting_human'" flat dense round icon="play_arrow" color="positive" @click="resumeExec">
        <q-tooltip>恢复执行</q-tooltip>
      </q-btn>
    </div>

    <div v-if="showProgressBar" class="graph-run-progress">
      <div class="graph-run-progress__bar">
        <div class="graph-run-progress__fill" :style="{ width: progressPercent + '%' }"></div>
        <div class="graph-run-progress__dot" :style="{ left: progressPercent + '%' }"></div>
      </div>
      <div class="graph-run-progress__stats">
        <span class="graph-run-progress__stat graph-run-progress__stat--completed">● {{ progressCompleted }} 完成</span>
        <span v-if="progressRunning > 0" class="graph-run-progress__stat graph-run-progress__stat--running">● {{ progressRunning }} 运行中</span>
        <span v-if="progressWaiting > 0" class="graph-run-progress__stat graph-run-progress__stat--waiting">○ {{ progressWaiting }} 等待</span>
        <span v-if="progressDurationSec" class="graph-run-progress__stat graph-run-progress__stat--duration">⏱ {{ progressDurationSec }}s</span>
      </div>
    </div>

    <div class="graph-workbench__body graph-run-page__body">
      <GraphEditorCanvas
        :graph-def="graphDef"
        :is-dark="isDark"
        :exec-node-states="execNodeStates"
        :selected-node-id="selectedNodeId"
        :focus-selected-node="true"
        :read-only="true"
        @select-node="onSelectNode"
        @update-graph="() => {}"
      />
      <GraphRunInspector
        :execution="execution"
        :execution-summary="executionSummary"
        :display-status="displayStatus"
        :status-color="statusColor"
        :stream-connected="streamConnected"
        :is-dark="isDark"
        :checkpoints="checkpoints"
        :checkpoints-loading="checkpointsLoading"
        :selected-checkpoint="selectedCheckpoint"
        :state-patch-json="statePatchJson"
        :snapshot-loading="snapshotLoading"
        :edit-loading="editLoading"
        :time-travel-loading="timeTravelLoading"
        :step-index="stepIndex"
        :tasks="taskList"
        :tasks-loading="tasksLoading"
        :selected-task-id="selectedTaskId"
        :tab="inspectorTab"
        @update:tab="inspectorTab = $event"
        @refresh-checkpoints="timeTravelLoadCheckpoints"
        @select-checkpoint="onSelectCheckpoint"
        @update:state-patch-json="updateStatePatchJson"
        @update:step-index="updateStepIndex"
        @time-travel="onTimeTravel"
        @apply-edit="onApplyEditState"
        @refresh-tasks="loadTasks"
        @select-task="openTaskDetail"
        @kanban-admin-action="onKanbanAdminAction"
      />
    </div>

    <GraphHitlDialog
      v-model:open="hitlDialogOpen"
      v-model:advanced-json="hitlAdvancedJson"
      :interrupt="interrupt"
      :loading="resumeLoading"
      @approve="submitHitlResume(true)"
      @dismiss="submitHitlResume(false)"
    />

    <GraphTaskDetailDrawer
      v-model:open="taskDrawerOpen"
      :task="activeTask"
      :comments="taskComments"
      :events="taskEvents"
      :logs="taskLogs"
      :runs="taskRuns"
      :detail-loading="taskDetailLoading"
      :action-loading="taskActionLoading"
      @claim="onClaimTask"
      @submit="onSubmitTask"
      @report-blocked="onReportBlocked"
      @unblock="onUnblockTask"
      @review="onReviewTask"
      @add-comment="onAddTaskComment"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import GraphEditorCanvas from "../components/graph/GraphEditorCanvas.vue";
import GraphRunInspector from "../components/graph/GraphRunInspector.vue";
import GraphHitlDialog from "../components/graph/GraphHitlDialog.vue";
import GraphTaskDetailDrawer from "../components/graph/GraphTaskDetailDrawer.vue";
import { useGraphRunPage } from "../features/graph/useGraphRunPage";

const {
  isDark,
  graphDef,
  execution,
  execNodeStates,
  executionSummary,
  streamConnected,
  interrupt,
  selectedNodeId,
  hitlDialogOpen,
  hitlAdvancedJson,
  resumeLoading,
  displayStatus,
  statusColor,
  taskList,
  tasksLoading,
  selectedTaskId,
  taskDrawerOpen,
  activeTask,
  taskComments,
  taskLogs,
  taskRuns,
  taskEvents,
  taskDetailLoading,
  taskActionLoading,
  checkpoints,
  checkpointsLoading,
  selectedCheckpoint,
  statePatchJson,
  snapshotLoading,
  editLoading,
  timeTravelLoading,
  stepIndex,
  onSelectNode,
  confirmCancelExec,
  cancelExec,
  resumeExec,
  submitHitlResume,
  onSelectCheckpoint,
  onTimeTravel,
  onApplyEditState,
  loadTasks,
  timeTravelLoadCheckpoints,
  updateStatePatchJson,
  updateStepIndex,
  openTaskDetail,
  onClaimTask,
  onSubmitTask,
  onReportBlocked,
  onUnblockTask,
  onReviewTask,
  onAddTaskComment,
  onKanbanAdminAction,
  inspectorTab,
  stepIcon,
  stepColor,
  formatTime,
  goBack,
} = useGraphRunPage();

const progressCompleted = computed(() => {
  let count = 0;
  for (const state of execNodeStates.value.values()) {
    if (state.status === "completed") count++;
  }
  return count;
});

const progressRunning = computed(() => {
  let count = 0;
  for (const state of execNodeStates.value.values()) {
    if (state.status === "running") count++;
  }
  return count;
});

const progressWaiting = computed(() => {
  let count = 0;
  for (const state of execNodeStates.value.values()) {
    if (state.status === "waiting" || state.status === "idle") count++;
  }
  return count;
});

const progressTotal = computed(() => {
  const fromStates = execNodeStates.value.size;
  const fromDef = graphDef.nodes?.length ?? 0;
  return Math.max(fromStates, fromDef);
});

const progressPercent = computed(() => {
  if (progressTotal.value === 0) return 0;
  return Math.round((progressCompleted.value / progressTotal.value) * 100);
});

const progressStepLabel = computed(() => {
  if (executionSummary.value?.totalSteps) {
    return `Step ${progressCompleted.value}/${executionSummary.value.totalSteps}`;
  }
  if (progressTotal.value > 0) {
    return `Step ${progressCompleted.value}/${progressTotal.value}`;
  }
  return "";
});

const progressDurationSec = computed(() => {
  const ms = executionSummary.value?.durationMs;
  if (ms && ms > 0) return (ms / 1000).toFixed(1);
  return "";
});

const showProgressBar = computed(() => execNodeStates.value.size > 0);
</script>
