<template>
  <div :class="['graph-run-inspector', { 'is-dark': isDark }]">
    <q-tabs v-model="tab" dense align="left" class="graph-run-inspector__tabs" active-color="primary">
      <q-tab name="overview" label="监控" />
      <q-tab name="checkpoints" label="检查点" />
      <q-tab name="tasks" label="任务" />
    </q-tabs>
    <q-separator />
    <q-tab-panels v-model="tab" animated class="graph-run-inspector__panels">
      <q-tab-panel name="overview" class="q-pa-none">
        <GraphRunSidebar
          :execution="execution"
          :execution-summary="executionSummary"
          :display-status="displayStatus"
          :status-color="statusColor"
          :stream-connected="streamConnected"
          :is-dark="isDark"
          embedded
        />
      </q-tab-panel>
      <q-tab-panel name="checkpoints" class="q-pa-md">
        <GraphCheckpointPanel
          :checkpoints="checkpoints"
          :loading="checkpointsLoading"
          :selected-checkpoint-id="selectedCheckpoint?.checkpointId"
          :state-snapshot="stateSnapshot"
          :restoring="restoringCheckpoint"
          @refresh="$emit('refreshCheckpoints')"
          @select="$emit('selectCheckpoint', $event)"
          @restore="$emit('restoreCheckpoint', $event)"
        />
        <GraphTimeTravelPanel
          :selected-checkpoint="selectedCheckpoint"
          :state-patch-json="statePatchJson"
          :snapshot-loading="snapshotLoading"
          :edit-loading="editLoading"
          :time-travel-loading="timeTravelLoading"
          :step-index="stepIndex"
          :max-step="maxStep"
          @update:state-patch-json="$emit('update:statePatchJson', $event)"
          @update:step-index="$emit('update:stepIndex', $event)"
          @time-travel="$emit('timeTravel')"
          @apply-edit="$emit('applyEdit')"
        />
      </q-tab-panel>
      <q-tab-panel name="tasks" class="q-pa-md">
        <GraphTaskKanban
          :tasks="tasks"
          :loading="tasksLoading"
          :live-connected="streamConnected"
          :selected-task-id="selectedTaskId"
          :is-dark="isDark"
          admin-drag
          @refresh="$emit('refreshTasks')"
          @select-task="$emit('selectTask', $event)"
          @admin-action="$emit('kanbanAdminAction', $event)"
        />
      </q-tab-panel>
    </q-tab-panels>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import GraphRunSidebar from './GraphRunSidebar.vue';
import GraphCheckpointPanel from './GraphCheckpointPanel.vue';
import GraphTimeTravelPanel from './GraphTimeTravelPanel.vue';
import GraphTaskKanban from './GraphTaskKanban.vue';
import type { CheckpointInfo, GraphExecution, GraphRunExecutionSummary, Task } from '../../features/graph/types';

const props = defineProps<{
  execution: GraphExecution | null;
  executionSummary: GraphRunExecutionSummary | null;
  displayStatus: string;
  statusColor: string;
  streamConnected: boolean;
  isDark: boolean;
  checkpoints: CheckpointInfo[];
  checkpointsLoading: boolean;
  selectedCheckpoint: CheckpointInfo | null;
  stateSnapshot: Record<string, unknown> | null;
  statePatchJson: string;
  snapshotLoading: boolean;
  editLoading: boolean;
  timeTravelLoading: boolean;
  stepIndex: number;
  tasks: Task[];
  tasksLoading: boolean;
  selectedTaskId?: string | null;
  tab: string;
  restoringCheckpoint?: boolean;
}>();

const emit = defineEmits<{
  'update:tab': [value: string];
  refreshCheckpoints: [];
  selectCheckpoint: [checkpoint: CheckpointInfo];
  restoreCheckpoint: [checkpoint: CheckpointInfo];
  'update:statePatchJson': [value: string];
  'update:stepIndex': [value: number];
  timeTravel: [];
  applyEdit: [];
  refreshTasks: [];
  selectTask: [taskId: string];
  kanbanAdminAction: [payload: { taskId: string; action: 'unblock' | 'approve' }];
}>();

const tab = computed({
  get: () => props.tab,
  set: (value: string) => emit('update:tab', value),
});

const maxStep = computed(() => {
  const fromSteps = props.execution?.steps?.length ?? 0;
  const fromCheckpoints = props.checkpoints.length;
  return Math.max(0, Math.max(fromSteps, fromCheckpoints) - 1);
});
</script>
