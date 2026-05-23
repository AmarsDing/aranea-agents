<template>
  <q-card flat bordered :class="['orch-kanban-card', { 'is-dark': isDark, 'orch-kanban-card--selected': selected }]" @click="$emit('select')">
    <q-card-section class="row items-start justify-between no-wrap q-pb-sm">
      <div class="col min-width-0">
        <div class="text-weight-medium">{{ state.agent_name || state.agent_key || state.node_id }}</div>
        <div class="text-caption text-grey-7">{{ state.role || "worker" }}</div>
      </div>
      <OrchestrationStatusChip :display-status="state.display_status" :fine-status="state.status" />
    </q-card-section>
    <q-separator />
    <q-card-section class="q-gutter-sm orch-kanban-card__cols">
      <div class="orch-kanban-col">
        <div class="orch-kanban-col__title">收到</div>
        <div class="orch-kanban-col__body">{{ state.input_preview || "—" }}</div>
      </div>
      <div class="orch-kanban-col">
        <div class="orch-kanban-col__title">进行中</div>
        <div v-if="activityTimeline.length" class="orch-kanban-col__body orch-kanban-col__timeline">
          <div v-for="(item, idx) in activityTimeline" :key="idx" class="orch-kanban-timeline-item">
            <span>{{ item.display_label || item.tool_name || item.kind || "activity" }}</span>
            <span v-if="item.status" class="text-caption"> · {{ item.status }}</span>
          </div>
        </div>
        <div v-else-if="state.current_activity" class="orch-kanban-col__body">
          {{ state.current_activity.display_label || state.current_activity.tool_name || state.current_activity.kind }}
          <span v-if="state.current_activity.status" class="text-caption"> · {{ state.current_activity.status }}</span>
        </div>
        <div v-else-if="state.phase === 'doing'" class="orch-kanban-col__body text-grey-7">处理中…</div>
        <div v-else class="orch-kanban-col__body text-grey-7">—</div>
      </div>
      <div class="orch-kanban-col">
        <div class="orch-kanban-col__title">已交付</div>
        <div class="orch-kanban-col__body">{{ state.output_preview || state.error_message || "—" }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ActivitySnapshot, AgentNodeState } from "../../features/orchestration/types";
import OrchestrationStatusChip from "./OrchestrationStatusChip.vue";

const props = defineProps<{
  state: AgentNodeState;
  isDark: boolean;
  selected?: boolean;
}>();

defineEmits<{ select: [] }>();

const activityTimeline = computed(() => {
  const history = props.state.activity_history ?? [];
  if (history.length) return history.slice(-5);
  return props.state.current_activity ? [props.state.current_activity] : [];
});
</script>

<style scoped>
.orch-kanban-card {
  border-radius: 14px;
  cursor: pointer;
  transition: box-shadow 0.2s;
}
.orch-kanban-card--selected {
  box-shadow: 0 0 0 2px var(--color-accent, var(--q-primary));
}
.orch-kanban-card__cols {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.orch-kanban-col__title {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  opacity: 0.65;
  margin-bottom: 4px;
}
.orch-kanban-col__body {
  font-size: 12px;
  line-height: 1.4;
  word-break: break-word;
  min-height: 2.5em;
}
.orch-kanban-col__timeline {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.orch-kanban-timeline-item {
  padding-left: 8px;
  border-left: 2px solid color-mix(in srgb, var(--q-primary) 35%, transparent);
}
@media (max-width: 900px) {
  .orch-kanban-card__cols {
    grid-template-columns: 1fr;
  }
}
</style>
