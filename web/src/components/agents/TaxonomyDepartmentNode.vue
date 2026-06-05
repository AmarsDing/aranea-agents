<template>
  <q-expansion-item
    :model-value="expanded"
    expand-icon="keyboard_arrow_down"
    class="taxonomy-tree-department"
    @update:model-value="emit('update:expanded', $event)"
  >
    <template #header>
      <taxonomy-node-header
        :node="department"
        :show-system-chip="showSystemChip"
        :readonly="readonly"
        :toggle-loading="toggleLoading"
        @edit="emit('edit', department)"
        @create-child="emit('create-child')"
        @remove="emit('remove', department)"
        @toggle-enabled="emit('toggle-enabled', department, $event)"
      />
    </template>

    <div class="position-card-grid">
      <draggable
        :model-value="resolvedPositions"
        item-key="id"
        :disabled="readonly"
        class="position-card-grid__draggable"
        @update:model-value="emit('reorder-positions', $event)"
      >
        <template #item="{ element: pos }">
          <taxonomy-position-card
            :position="pos"
            :path="path"
            :readonly="readonly"
            :highlight="positionHighlighted(pos)"
            @edit="emit('edit', $event)"
            @remove="emit('remove', $event)"
          />
        </template>
      </draggable>

      <button
        v-if="!readonly"
        type="button"
        class="position-card-add"
        @click="emit('create-child')"
      >
        <q-icon name="add" size="22px" color="primary" />
        <span>新增职位</span>
      </button>
    </div>
  </q-expansion-item>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import draggable from 'vuedraggable';
import TaxonomyPositionCard from './TaxonomyPositionCard.vue';
import TaxonomyNodeHeader from './TaxonomyNodeHeader.vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import { departmentPositions, nodeMatchesKeyword } from '../../features/platform/taxonomyTreeUtils';

const props = withDefaults(
  defineProps<{
    department: PlatformResourceTreeNode;
    positions?: PlatformResourceTreeNode[];
    path: string;
    expanded?: boolean;
    readonly?: boolean;
    showSystemChip?: boolean;
    toggleLoading?: boolean;
    keyword?: string;
  }>(),
  {
    positions: undefined,
    expanded: false,
    readonly: false,
    showSystemChip: true,
    toggleLoading: false,
    keyword: '',
  },
);

const emit = defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  'create-child': [];
  remove: [node: PlatformResourceTreeNode];
  'toggle-enabled': [node: PlatformResourceTreeNode, enabled: boolean];
  'update:expanded': [value: boolean];
  'reorder-positions': [positions: PlatformResourceTreeNode[]];
}>();

const resolvedPositions = computed(() =>
  props.positions ?? departmentPositions(props.department),
);

function positionHighlighted(position: PlatformResourceTreeNode) {
  return nodeMatchesKeyword(position, props.keyword);
}
</script>
