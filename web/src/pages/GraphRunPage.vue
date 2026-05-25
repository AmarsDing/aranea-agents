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
      <q-btn v-if="displayStatus === 'running'" flat dense round icon="stop" color="negative" @click="cancelExec">
        <q-tooltip>取消执行</q-tooltip>
      </q-btn>
      <q-btn v-if="displayStatus === 'waiting_human'" flat dense round icon="play_arrow" color="positive" @click="resumeExec">
        <q-tooltip>恢复执行</q-tooltip>
      </q-btn>
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
        :format-time="formatTime"
        :step-icon="stepIcon"
        :step-color="stepColor"
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
</script>
