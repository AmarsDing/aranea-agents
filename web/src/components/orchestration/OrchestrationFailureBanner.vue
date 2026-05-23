<template>
  <q-banner v-if="visible" dense rounded class="orch-failure-banner" role="alert">
    <template #avatar>
      <q-icon :name="iconName" :color="iconColor" />
    </template>
    <div class="orch-failure-banner__title">{{ title }}</div>
    <div class="text-caption">{{ message }}</div>
    <div v-if="circuitOpen" class="orch-failure-banner__circuit row items-center q-mt-xs q-gutter-xs">
      <q-badge color="negative" outline>熔断已打开</q-badge>
      <span class="text-caption">连续失败达阈值，节点 {{ primary?.node_id }} 已阻断</span>
    </div>
    <template #action>
      <q-btn v-if="canRetry" flat dense color="primary" label="重试" @click="$emit('retry', primary?.node_id)" />
      <q-btn v-if="canFallback" flat dense color="warning" label="切 fallback" @click="$emit('fallback', primary?.node_id)" />
      <q-btn v-if="canReview" flat dense color="primary" label="审核" @click="$emit('review', primary?.node_id)" />
      <q-btn flat dense color="grey-7" label="终止" @click="$emit('halt')" />
    </template>
  </q-banner>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AgentNodeState } from "../../features/orchestration/types";

const props = defineProps<{
  nodes: AgentNodeState[];
  runStatus?: string;
}>();

defineEmits<{ retry: [nodeId?: string]; fallback: [nodeId?: string]; review: [nodeId?: string]; halt: [] }>();

const failedNodes = computed(() =>
  props.nodes.filter(
    (n) =>
      n.status === "failed" ||
      n.status === "waiting_review" ||
      n.status === "blocked" ||
      n.display_status === "failed" ||
      isCircuitOpen(n),
  ),
);

function isCircuitOpen(n: AgentNodeState): boolean {
  if (n.status === "blocked") return true;
  const msg = String(n.error_message ?? "").toLowerCase();
  return msg.includes("circuit") || msg.includes("熔断");
}

const visible = computed(() => failedNodes.value.length > 0 || props.runStatus === "failed");

const primary = computed(() => failedNodes.value[0]);

const circuitOpen = computed(() => Boolean(primary.value && isCircuitOpen(primary.value)));

const title = computed(() => {
  if (circuitOpen.value) return "熔断器已打开 (FP-02)";
  if (primary.value?.status === "waiting_review") return "等待人工审核 (HITL)";
  if (primary.value?.status === "failed") return "节点执行失败";
  return "编排运行异常";
});

const message = computed(() => {
  if (circuitOpen.value) {
    return primary.value?.error_message || "节点因连续失败被熔断阻断，可重试或切换 fallback。";
  }
  return primary.value?.error_message || primary.value?.output_preview || "请查看 Kanban / Graph 详情。";
});

const iconName = computed(() => {
  if (circuitOpen.value) return "bolt";
  if (primary.value?.status === "waiting_review") return "hourglass_empty";
  return "error";
});

const iconColor = computed(() => (circuitOpen.value ? "orange" : "negative"));

const canReview = computed(() => primary.value?.status === "waiting_review");
const canRetry = computed(() => primary.value?.status === "failed" || circuitOpen.value);
const canFallback = computed(() => primary.value?.status === "failed" || circuitOpen.value || primary.value?.status === "waiting_review");
</script>

<style scoped>
.orch-failure-banner {
  border: 1px solid color-mix(in srgb, var(--state-danger, #c62828) 35%, transparent);
  background: color-mix(in srgb, var(--state-danger, #c62828) 8%, transparent);
  margin-bottom: 8px;
}
.orch-failure-banner__title {
  font-weight: 600;
}
.orch-failure-banner__circuit {
  color: var(--color-text-secondary);
}
</style>
