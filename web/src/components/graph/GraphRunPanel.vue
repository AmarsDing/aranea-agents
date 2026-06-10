<template>
  <q-drawer
    :model-value="visible"
    :width="panelWidth"
    :mini="false"
    side="right"
    bordered
    :class="['graph-run-panel', { 'is-dark': isDark }]"
    @update:model-value="(v: boolean) => $emit('update:visible', v)"
  >
    <!-- Header -->
    <div class="graph-run-panel__header">
      <div class="graph-run-panel__title">运行面板</div>
      <q-btn flat dense round icon="close" size="sm" @click="$emit('update:visible', false)" />
    </div>
    <q-separator />

    <!-- Execution controls -->
    <div class="graph-run-panel__controls q-pa-sm">
      <q-btn
        v-if="!isRunning"
        flat
        dense
        no-caps
        color="positive"
        icon="play_arrow"
        label="执行"
        :loading="executing"
        @click="$emit('execute')"
      />
      <q-btn v-else flat dense no-caps color="negative" icon="stop" label="停止" @click="$emit('cancel')" />
      <q-chip v-if="isRunning" dense square class="graph-run-panel__running-chip q-ml-sm">
        <q-icon name="sync" size="14px" class="q-mr-xs" /> 运行中
      </q-chip>
      <q-chip v-else-if="lastStatus === 'completed'" dense square color="positive" text-color="white" class="q-ml-sm">
        完成
      </q-chip>
      <q-chip v-else-if="lastStatus === 'failed'" dense square color="negative" text-color="white" class="q-ml-sm">
        失败
      </q-chip>
    </div>

    <q-separator />

    <!-- Tab panels: State / Checkpoints / HITL -->
    <q-tabs v-model="activeTab" dense inline-label class="graph-run-panel__tabs">
      <q-tab name="state" icon="data_object" label="State" />
      <q-tab name="checkpoints" icon="history" label="检查点" />
      <q-tab name="hitl" icon="front_hand" label="审批" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" class="graph-run-panel__panels">
      <!-- State View -->
      <q-tab-panel name="state">
        <div v-if="!currentState || Object.keys(currentState).length === 0" class="graph-run-panel__empty">
          <q-icon name="data_object" size="32px" color="grey-6" />
          <div class="text-caption text-grey-7 q-mt-sm">执行后查看 State 快照</div>
        </div>
        <div v-else class="graph-run-panel__state-view">
          <div v-for="(value, key) in currentState" :key="String(key)" class="graph-run-panel__state-field">
            <div class="graph-run-panel__state-key">{{ String(key) }}</div>
            <div class="graph-run-panel__state-value">
              <pre>{{ typeof value === 'string' ? value : JSON.stringify(value, null, 2) }}</pre>
            </div>
          </div>
        </div>
      </q-tab-panel>

      <!-- Checkpoints -->
      <q-tab-panel name="checkpoints">
        <div v-if="!checkpoints || checkpoints.length === 0" class="graph-run-panel__empty">
          <q-icon name="history" size="32px" color="grey-6" />
          <div class="text-caption text-grey-7 q-mt-sm">执行后查看检查点</div>
        </div>
        <div v-else class="graph-run-panel__checkpoint-list">
          <div
            v-for="cp in checkpoints"
            :key="cp.checkpointId"
            class="graph-run-panel__checkpoint-item"
            :class="{ 'graph-run-panel__checkpoint-item--active': cp.step === currentStep }"
            @click="$emit('timeTravel', cp)"
          >
            <div class="graph-run-panel__checkpoint-step">Step {{ cp.step }}</div>
            <div class="graph-run-panel__checkpoint-id">{{ cp.checkpointId.slice(0, 8) }}</div>
          </div>
        </div>
      </q-tab-panel>

      <!-- HITL Queue -->
      <q-tab-panel name="hitl">
        <div v-if="!hitlItems || hitlItems.length === 0" class="graph-run-panel__empty">
          <q-icon name="front_hand" size="32px" color="grey-6" />
          <div class="text-caption text-grey-7 q-mt-sm">无待审批项</div>
        </div>
        <div v-else class="graph-run-panel__hitl-list">
          <div v-for="item in hitlItems" :key="item.taskId" class="graph-run-panel__hitl-item">
            <div class="graph-run-panel__hitl-node">节点: {{ item.nodeId }}</div>
            <div class="graph-run-panel__hitl-status">{{ item.status }}</div>
            <q-btn flat dense color="primary" label="审批" size="sm" @click="$emit('handleHitl', item)" />
          </div>
        </div>
      </q-tab-panel>
    </q-tab-panels>
  </q-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { CheckpointInfo, Task } from '../../features/graph/types';

defineProps<{
  visible: boolean;
  isDark: boolean;
  isRunning: boolean;
  executing: boolean;
  lastStatus: string;
  currentState: Record<string, unknown> | null;
  checkpoints: CheckpointInfo[];
  currentStep: number;
  hitlItems: Task[];
}>();

defineEmits<{
  'update:visible': [value: boolean];
  execute: [];
  cancel: [];
  timeTravel: [checkpoint: CheckpointInfo];
  handleHitl: [task: Task];
}>();

const activeTab = ref('state');
const panelWidth = 326;
</script>
