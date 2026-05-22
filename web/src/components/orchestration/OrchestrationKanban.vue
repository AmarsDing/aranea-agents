<template>
  <div :class="['orch-kanban', { 'is-dark': isDark }]">
    <div class="orch-kanban__header row items-center q-gutter-sm q-mb-md">
      <div class="text-subtitle2">Agent 工作看板</div>
      <q-badge v-if="liveConnected" rounded color="positive">实时</q-badge>
      <q-space />
      <q-select
        v-model="filterDisplay"
        dense
        outlined
        emit-value
        map-options
        clearable
        label="状态筛选"
        class="orch-kanban__filter"
        :options="filterOptions"
      />
    </div>
    <div v-if="sortedNodes.length === 0" class="text-caption text-grey-7 q-pa-md">暂无 Agent 节点。</div>
    <div v-else class="orch-kanban__list q-gutter-md">
      <OrchestrationKanbanCard
        v-for="node in sortedNodes"
        :key="node.node_id"
        :ref="(el) => setCardRef(node.node_id, el)"
        :state="node"
        :is-dark="isDark"
        :selected="node.node_id === selectedNodeId"
        @select="$emit('selectNode', node.node_id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, type ComponentPublicInstance } from "vue";
import type { AgentNodeState, DisplayStatus } from "../../features/orchestration/types";
import { DISPLAY_STATUS_STYLES } from "../../features/orchestration/agentNodeStatusStyles";
import OrchestrationKanbanCard from "./OrchestrationKanbanCard.vue";

const props = defineProps<{
  nodes: AgentNodeState[];
  isDark: boolean;
  liveConnected?: boolean;
  selectedNodeId?: string | null;
}>();

defineEmits<{ selectNode: [nodeId: string] }>();

const filterDisplay = ref<DisplayStatus | null>(null);
const cardRefs = ref(new Map<string, HTMLElement>());

function setCardRef(nodeId: string, el: Element | ComponentPublicInstance | null) {
  const root = (el as ComponentPublicInstance | null)?.$el ?? el;
  if (root instanceof HTMLElement) {
    cardRefs.value.set(nodeId, root);
  } else {
    cardRefs.value.delete(nodeId);
  }
}

watch(
  () => props.selectedNodeId,
  (nodeId) => {
    if (!nodeId) return;
    nextTick(() => {
      cardRefs.value.get(nodeId)?.scrollIntoView({ behavior: "smooth", block: "nearest" });
    });
  }
);

const filterOptions = Object.entries(DISPLAY_STATUS_STYLES).map(([value, cfg]) => ({
  label: cfg.label,
  value,
}));

const sortedNodes = computed(() => {
  let list = [...props.nodes];
  if (filterDisplay.value) {
    list = list.filter((n) => n.display_status === filterDisplay.value);
  }
  return list.sort((a, b) => a.node_id.localeCompare(b.node_id));
});
</script>

<style scoped>
.orch-kanban__filter {
  min-width: 140px;
}
</style>
