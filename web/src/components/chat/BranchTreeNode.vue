<template>
  <div class="branch-tree-node">
    <q-item
      clickable
      dense
      :active="selectedId === node.id"
      active-class="branch-tree-node--active"
      @click="$emit('select', node.id)"
    >
      <q-item-section>
        <q-item-label>{{ node.type }}</q-item-label>
        <q-item-label caption>{{ node.author }} · {{ shortTime(node.timestamp) }}</q-item-label>
      </q-item-section>
    </q-item>
    <div v-if="node.children.length" class="branch-tree-node__children q-pl-md">
      <BranchTreeNode
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :selected-id="selectedId"
        @select="$emit('select', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { BranchNode } from "../../features/chat/eventFilter";

defineProps<{
  node: BranchNode;
  selectedId?: string | null;
}>();

defineEmits<{
  select: [invocationId: string];
}>();

function shortTime(ts: string): string {
  if (!ts) return "";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return d.toLocaleTimeString();
}
</script>

<style scoped>
.branch-tree-node--active {
  background: rgba(25, 118, 210, 0.08);
  border-radius: 8px;
}
</style>
