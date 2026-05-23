<template>
  <div class="orch-activity-timeline">
    <div v-if="showToolbar" class="orch-activity-timeline__toolbar row items-center q-mb-sm q-gutter-sm">
      <q-badge color="primary" outline>GetTeamRunObservatoryTimeline RPC</q-badge>
      <q-select
        v-if="nodeFilterOptions.length"
        :model-value="nodeFilter"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="节点筛选"
        class="col"
        :options="nodeFilterOptions"
        @update:model-value="$emit('update:nodeFilter', $event)"
      />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="$emit('refresh')">
        <q-tooltip>刷新 Timeline</q-tooltip>
      </q-btn>
    </div>
    <div v-if="loading" class="flex flex-center q-pa-md">
      <q-spinner color="primary" size="28px" />
    </div>
    <div v-else-if="!rows.length" class="text-caption text-grey-7 q-pa-sm">
      暂无 Activity 记录；运行期间工具调用会写入时间线。
    </div>
    <q-list v-else dense separator class="orch-activity-timeline__list">
      <q-item
        v-for="(row, idx) in rows"
        :key="`${row.node_id}-${row.started_at}-${idx}`"
        clickable
        @click="$emit('select-node', row.node_id)"
      >
        <q-item-section avatar>
          <q-icon name="timeline" color="primary" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ row.display_label || row.kind || "activity" }}</q-item-label>
          <q-item-label caption>
            {{ row.node_id }} · {{ row.status || "—" }}
            <span v-if="row.duration_ms"> · {{ row.duration_ms }}ms</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="text-caption text-grey-7">{{ formatTime(row.started_at) }}</div>
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import type { ActivityTimelineRow } from "../../features/orchestration/types";

withDefaults(
  defineProps<{
    rows: ActivityTimelineRow[];
    loading: boolean;
    nodeFilter?: string | null;
    nodeFilterOptions?: Array<{ label: string; value: string }>;
    showToolbar?: boolean;
  }>(),
  { nodeFilter: null, nodeFilterOptions: () => [], showToolbar: true },
);

defineEmits<{ "select-node": [nodeId: string]; refresh: []; "update:nodeFilter": [value: string | null] }>();

function formatTime(value: string) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleTimeString();
}
</script>

<style scoped>
.orch-activity-timeline__list {
  border-radius: 12px;
  overflow: hidden;
}

.orch-activity-timeline__toolbar {
  flex-wrap: wrap;
}
</style>
