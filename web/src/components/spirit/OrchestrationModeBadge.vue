<template>
  <div v-if="topology" class="orchestration-mode-badge" :class="`orchestration-mode-badge--${topology}`">
    <q-icon :name="topologyIcon" size="14px" class="q-mr-xs" />
    <span class="orchestration-mode-badge__label">{{ topologyLabel }}</span>
    <q-tooltip v-if="reason" :delay="300">{{ reason }}</q-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { TopologyType } from "../../features/spirit/types";

const props = defineProps<{
  topology: TopologyType;
  reason?: string;
}>();

const topologyLabel = computed(() => {
  const labels: Record<TopologyType, string> = {
    parallel: "并行",
    sequential: "顺序",
    hybrid: "混合",
    coordinator: "协调",
  };
  return labels[props.topology] ?? props.topology;
});

const topologyIcon = computed(() => {
  const icons: Record<TopologyType, string> = {
    parallel: "vertical_align_center",
    sequential: "format_list_numbered",
    hybrid: "account_tree",
    coordinator: "hub",
  };
  return icons[props.topology] ?? "hub";
});
</script>

<style scoped lang="sass">
.orchestration-mode-badge
  display: inline-flex
  align-items: center
  padding: 2px 8px
  border-radius: 6px
  font-size: 11px
  font-weight: 600
  border: 1px solid transparent
  cursor: default

.orchestration-mode-badge--parallel
  color: var(--color-accent)
  background: color-mix(in srgb, var(--color-accent) 8%, transparent)
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent)

.orchestration-mode-badge--sequential
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent)
  border-color: var(--glass-border)

.orchestration-mode-badge--hybrid
  color: var(--color-warning)
  background: color-mix(in srgb, var(--color-warning) 8%, transparent)
  border-color: color-mix(in srgb, var(--color-warning) 20%, transparent)

.orchestration-mode-badge--coordinator
  color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 8%, transparent)
  border-color: color-mix(in srgb, var(--color-success) 20%, transparent)

.orchestration-mode-badge__label
  line-height: 1.4
</style>
