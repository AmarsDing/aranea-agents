<template>
  <div class="session-tree-node">
    <div
      class="node-header row items-center no-wrap"
      :class="{ 'node-header--active': activeSessionId === node.session.id }"
      @click="onSelect"
    >
      <q-icon
        v-if="hasChildren"
        :name="expanded ? 'expand_more' : 'chevron_right'"
        size="16px"
        class="node-toggle"
        @click.stop="toggleExpand"
      />
      <span v-else class="node-toggle-placeholder" />
      <q-icon :name="sessionIcon" size="16px" class="node-icon" />
      <span class="node-title col ellipsis">{{ node.session.title || 'Untitled' }}</span>
      <q-badge v-if="hasChildren" dense rounded class="node-count">
        {{ node.children.length }}
      </q-badge>
    </div>
    <div v-if="expanded && hasChildren" class="node-children">
      <SessionTreeNode
        v-for="child in node.children"
        :key="child.session.id"
        :node="child"
        :active-session-id="activeSessionId"
        :default-expanded="defaultExpanded"
        @select="emit('select', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { SessionTreeNode } from '../../features/session/types';

const props = defineProps<{
  node: SessionTreeNode;
  activeSessionId: string;
  defaultExpanded?: boolean;
}>();

const emit = defineEmits<{
  select: [sessionId: string];
}>();

const expanded = ref(props.defaultExpanded ?? false);

// Auto-expand when the active session is this node or a descendant.
watch(
  () => props.activeSessionId,
  (activeId) => {
    if (activeId && containsSession(props.node, activeId)) {
      expanded.value = true;
    }
  },
  { immediate: true },
);

const hasChildren = computed(() => props.node.children.length > 0);

function toggleExpand() {
  expanded.value = !expanded.value;
}

function onSelect() {
  emit('select', props.node.session.id);
}

const sessionIcon = computed(() => {
  const s = props.node.session;
  if (s.team_id) return 'groups';
  if (s.agent_id) return 'person';
  return 'forum';
});

function containsSession(node: SessionTreeNode, sessionId: string): boolean {
  if (node.session.id === sessionId) return true;
  return node.children.some((c) => containsSession(c, sessionId));
}
</script>

<style lang="sass" scoped>
.session-tree-node
  .node-header
    padding: 4px 8px
    border-radius: 6px
    cursor: pointer
    gap: 4px
    user-select: none
    &:hover
      background: var(--glass-surface)
    &--active
      background: var(--color-accent-transparent, rgba(99, 102, 241, 0.12))

  .node-toggle
    color: var(--color-text-secondary)
    cursor: pointer
    flex-shrink: 0

  .node-toggle-placeholder
    width: 16px
    flex-shrink: 0

  .node-icon
    color: var(--color-text-secondary)
    flex-shrink: 0

  .node-title
    font-size: 13px
    color: var(--color-text-primary)

  .node-count
    background: var(--glass-surface)
    color: var(--color-text-secondary)
    font-size: 10px

  .node-children
    margin-left: 16px
    border-left: 1px solid var(--glass-border)
    padding-left: 4px
</style>
