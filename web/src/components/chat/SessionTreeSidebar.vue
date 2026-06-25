<template>
  <div class="session-tree-sidebar">
    <div v-if="loading" class="session-tree-sidebar__state row items-center justify-center">
      <q-spinner color="accent" size="24px" />
    </div>
    <div v-else-if="error" class="session-tree-sidebar__state session-tree-sidebar__state--error">
      {{ error }}
    </div>
    <div v-else-if="!treeNodes.length" class="session-tree-sidebar__state">
      <q-icon name="account_tree" size="28px" class="text-cream-muted" />
      <div class="q-mt-sm text-cream-muted text-caption">No session tree</div>
    </div>
    <div v-else class="session-tree-sidebar__tree">
      <SessionTreeNode
        v-for="node in treeNodes"
        :key="node.session.id"
        :node="node"
        :active-session-id="activeSessionId"
        :default-expanded="defaultExpanded"
        @select="emit('select', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SessionTreeNode as SessionTreeNodeData } from '../../features/session/types';
import SessionTreeNode from './SessionTreeNode.vue';

defineProps<{
  treeNodes: SessionTreeNodeData[];
  activeSessionId: string;
  loading?: boolean;
  error?: string;
  defaultExpanded?: boolean;
}>();

const emit = defineEmits<{
  select: [sessionId: string];
}>();
</script>

<style lang="sass" scoped>
.session-tree-sidebar
  &__state
    padding: 24px 16px
    text-align: center
    color: var(--color-text-secondary)
    flex-direction: column

    &--error
      color: var(--color-danger)

  &__tree
    padding: 4px
</style>
