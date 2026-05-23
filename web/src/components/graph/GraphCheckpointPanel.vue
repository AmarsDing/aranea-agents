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

defineProps<{
  checkpoints: CheckpointInfo[];
  loading: boolean;
  selectedCheckpointId?: string | null;
}>();

defineEmits<{
  refresh: [];
  select: [checkpoint: CheckpointInfo];
}>();

function formatTime(ts: string) {
  if (!ts) return "—";
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}
</script>

<style scoped>
.graph-checkpoint-panel__title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, var(--color-text-secondary));
}

.graph-checkpoint-panel__mono {
  font-family: monospace;
  font-size: 11px;
  word-break: break-all;
}

.graph-checkpoint-panel__item--active {
  background: rgb(233 162 59 / 12%);
}
</style>
