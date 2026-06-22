<template>
  <div :class="['orch-kanban', { 'is-dark': isDark }]">
    <div class="orch-kanban__header row items-center q-gutter-sm q-mb-md">
      <div class="text-subtitle2">Agent 工作看板</div>
      <q-badge v-if="liveConnected" rounded class="graph-kanban-live">实时</q-badge>
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

    <div v-if="filteredNodes.length === 0" class="orch-kanban__empty">
      {{ emptyLabel }}
    </div>
    <div v-else class="orch-kanban__list">
      <OrchestrationKanbanCard
        v-for="node in filteredNodes"
        :key="node.node_id"
        :ref="(el) => setCardRef(node.node_id, el)"
        :state="node"
        :is-dark="isDark"
        :selected="node.node_id === selectedNodeId"
        class="q-mb-sm"
        @select="$emit('selectNode', node.node_id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, type ComponentPublicInstance } from 'vue';
import type { AgentNodeState, DisplayStatus } from '../../features/orchestration/types';
import { DISPLAY_STATUS_STYLES } from '../../features/orchestration/agentNodeStatusStyles';
import OrchestrationKanbanCard from './OrchestrationKanbanCard.vue';

const props = withDefaults(
  defineProps<{
    nodes: AgentNodeState[];
    isDark: boolean;
    liveConnected?: boolean;
    selectedNodeId?: string | null;
    emptyLabel?: string;
  }>(),
  { emptyLabel: '暂无 Agent', selectedNodeId: null },
);

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
      cardRefs.value.get(nodeId)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    });
  },
);

const filterOptions = Object.entries(DISPLAY_STATUS_STYLES).map(([value, cfg]) => ({
  label: cfg.label,
  value,
}));

const filteredNodes = computed(() => {
  let list = [...props.nodes];
  if (filterDisplay.value) {
    list = list.filter((node) => node.display_status === filterDisplay.value);
  }
  return list.sort((a, b) => a.node_id.localeCompare(b.node_id));
});
</script>
