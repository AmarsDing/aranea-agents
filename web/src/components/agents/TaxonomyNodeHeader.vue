<template>
  <div class="taxonomy-tree-node row items-center no-wrap full-width">
    <q-icon :name="icon" color="primary" size="20px" class="q-mr-sm" />
    <div class="col min-width-0">
      <div class="row items-center q-gutter-xs no-wrap">
        <span class="taxonomy-tree-node__label ellipsis">{{ node.name }}</span>
        <q-chip
          v-if="showSystemChip"
          dense
          square
          size="sm"
          :class="parseIsSystem(node) ? 'system-chip' : 'custom-chip'"
        >
          {{ parseIsSystem(node) ? '系统' : '自建' }}
        </q-chip>
        <q-chip v-if="!node.enabled" dense square size="sm" class="taxonomy-tree-node__status-off">已停用</q-chip>
      </div>
      <div v-if="caption" class="taxonomy-tree-node__caption ellipsis">{{ caption }}</div>
    </div>
    <div v-if="!readonly" class="taxonomy-tree-node__actions row q-gutter-xs no-wrap items-center">
      <q-toggle
        :model-value="node.enabled"
        dense
        color="primary"
        checked-icon="check"
        unchecked-icon="close"
        :disable="toggleLoading"
        :aria-label="node.enabled ? `停用${levelLabel(node.level)}` : `启用${levelLabel(node.level)}`"
        @update:model-value="$emit('toggle-enabled', Boolean($event))"
        @click.stop
      />
      <q-btn flat dense round color="primary" icon="edit" @click.stop="$emit('edit')" />
      <q-btn
        v-if="node.level === 'industry'"
        flat
        dense
        rounded
        color="primary"
        icon="add"
        label="部门"
        @click.stop="$emit('create-child')"
      />
      <q-btn
        v-if="node.level === 'department'"
        flat
        dense
        rounded
        color="primary"
        icon="add"
        label="职位"
        @click.stop="$emit('create-child')"
      />
      <q-btn
        v-if="!parseIsSystem(node)"
        flat
        dense
        round
        color="negative"
        icon="delete"
        @click.stop="$emit('remove')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import { levelLabel, parseIsSystem, trimmedDesc } from '../../features/platform/taxonomyTreeUtils';

const props = defineProps<{
  node: PlatformResourceTreeNode;
  readonly?: boolean;
  showSystemChip?: boolean;
  toggleLoading?: boolean;
}>();

defineEmits<{
  edit: [];
  'create-child': [];
  remove: [];
  'toggle-enabled': [enabled: boolean];
}>();

const icon = computed(() => {
  if (props.node.level === 'industry') return 'domain';
  if (props.node.level === 'department') return 'lan';
  return 'badge';
});

const caption = computed(() => trimmedDesc(props.node.description));
</script>
