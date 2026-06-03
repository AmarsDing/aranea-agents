<template>
  <q-list dense class="branch-tree">
    <BranchTreeNode
      v-for="node in nodes"
      :key="node.id"
      :node="node"
      :selected-id="selectedId"
      @select="$emit('select', $event)"
    />
    <div v-if="!nodes.length" class="text-caption text-grey-7 q-pa-sm">暂无调用树</div>
  </q-list>
</template>

<script setup lang="ts">
import type { BranchNode } from '../../features/chat/eventFilter';
import BranchTreeNode from './BranchTreeNode.vue';

defineProps<{
  nodes: BranchNode[];
  selectedId?: string | null;
}>();

defineEmits<{
  select: [invocationId: string];
}>();
</script>
