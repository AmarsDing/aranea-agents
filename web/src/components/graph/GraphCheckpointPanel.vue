<template>
  <div class="graph-checkpoint-panel">
    <div class="graph-checkpoint-panel__header row items-center q-gutter-sm q-mb-sm">
      <div class="graph-checkpoint-panel__title">检查点</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="$emit('refresh')">
        <q-tooltip>刷新</q-tooltip>
      </q-btn>
    </div>
    <q-spinner v-if="loading" color="primary" size="28px" />
    <div v-else-if="!checkpoints.length" class="text-caption app-text-secondary">暂无检查点记录。</div>
    <q-list v-else dense bordered separator class="rounded-borders">
      <q-item
        v-for="cp in checkpoints"
        :key="cp.checkpointId"
        clickable
        :active="selectedCheckpointId === cp.checkpointId"
        active-class="graph-checkpoint-panel__item--active"
        @click="$emit('select', cp)"
      >
        <q-item-section>
          <q-item-label class="graph-checkpoint-panel__mono">{{ cp.checkpointId }}</q-item-label>
          <q-item-label caption>
            step {{ cp.step }} · {{ cp.source || "checkpoint" }}
          </q-item-label>
          <q-item-label caption>{{ formatTime(cp.timestamp) }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-badge rounded color="blue-grey">{{ cp.namespace || "default" }}</q-badge>
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import type { CheckpointInfo } from "../../features/graph/types";
import { formatTime } from "../../features/graph/utils";

defineProps<{
  checkpoints: CheckpointInfo[];
  loading: boolean;
  selectedCheckpointId?: string | null;
}>();

defineEmits<{
  refresh: [];
  select: [checkpoint: CheckpointInfo];
}>();


</script>
