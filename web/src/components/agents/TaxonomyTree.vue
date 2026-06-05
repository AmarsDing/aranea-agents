<template>
  <q-card flat bordered class="app-entity-glass-panel taxonomy-tree-panel">
    <q-card-section class="taxonomy-tree-panel__body">
      <q-list class="taxonomy-tree-list">
        <q-expansion-item
          v-for="industry in tree"
          :key="industry.id"
          :model-value="isExpanded(industry.id)"
          expand-icon="keyboard_arrow_down"
          class="taxonomy-tree-industry"
          @update:model-value="setExpanded(industry.id, $event)"
        >
          <template #header>
            <taxonomy-node-header
              :node="industry"
              :show-system-chip="showSystemChip"
              :readonly="readonly"
              :toggle-loading="isToggling(industry.id)"
              @edit="$emit('edit', industry)"
              @create-child="$emit('create-child', 'department', industry)"
              @remove="$emit('remove', industry)"
              @toggle-enabled="$emit('toggle-enabled', industry, $event)"
            />
          </template>

          <div class="taxonomy-tree-industry__body">
            <taxonomy-department-node
              v-for="department in departmentNodes(industry)"
              :key="department.id"
              :department="department"
              :positions="positionNodes(department)"
              :path="positionPath(industry, department)"
              :expanded="isExpanded(department.id)"
              :readonly="readonly"
              :show-system-chip="showSystemChip"
              :toggle-loading="isToggling(department.id)"
              :keyword="keyword"
              @edit="$emit('edit', $event)"
              @create-child="$emit('create-child', 'position', department)"
              @remove="$emit('remove', $event)"
              @toggle-enabled="$emit('toggle-enabled', department, $event)"
              @update:expanded="setExpanded(department.id, $event)"
              @reorder-positions="$emit('reorder-positions', department, $event)"
            />

            <q-btn
              v-if="!readonly"
              flat
              rounded
              color="primary"
              icon="add"
              label="新增部门"
              class="q-mt-sm q-ml-md"
              @click="$emit('create-child', 'department', industry)"
            />
          </div>
        </q-expansion-item>
      </q-list>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import TaxonomyDepartmentNode from './TaxonomyDepartmentNode.vue';
import TaxonomyNodeHeader from './TaxonomyNodeHeader.vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import {
  collectDefaultExpandedIds,
  collectExpandedIdsForFilter,
  departmentPositions,
  type TaxonomyLevel,
} from '../../features/platform/taxonomyTreeUtils';

const props = withDefaults(
  defineProps<{
    tree: PlatformResourceTreeNode[];
    keyword?: string;
    readonly?: boolean;
    showSystemChip?: boolean;
    defaultExpandAll?: boolean;
    togglingIds?: Set<string>;
  }>(),
  {
    keyword: '',
    readonly: false,
    showSystemChip: true,
    defaultExpandAll: true,
    togglingIds: () => new Set<string>(),
  },
);

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  'create-child': [level: TaxonomyLevel, parent: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
  'toggle-enabled': [node: PlatformResourceTreeNode, enabled: boolean];
  'reorder-positions': [department: PlatformResourceTreeNode, positions: PlatformResourceTreeNode[]];
}>();

const expandedIds = ref<Set<string>>(new Set());

watch(
  () => props.keyword,
  (keyword) => {
    const q = keyword.trim();
    if (!q) {
      expandedIds.value = collectDefaultExpandedIds(props.tree);
      return;
    }
    expandedIds.value = new Set(collectExpandedIdsForFilter(props.tree, q));
  },
);

watch(
  () => props.tree,
  (tree) => {
    if (!props.defaultExpandAll || !tree.length) return;
    if (expandedIds.value.size > 0) return;
    expandedIds.value = collectDefaultExpandedIds(tree);
  },
  { immediate: true, deep: false },
);

function isToggling(id: string) {
  return props.togglingIds.has(id);
}

function isExpanded(id: string) {
  return expandedIds.value.has(id);
}

function setExpanded(id: string, open: boolean) {
  const next = new Set(expandedIds.value);
  if (open) next.add(id);
  else next.delete(id);
  expandedIds.value = next;
}

function departmentNodes(industry: PlatformResourceTreeNode) {
  return (industry.children ?? []).filter((node) => node.level === 'department');
}

function positionNodes(department: PlatformResourceTreeNode) {
  return departmentPositions(department);
}

function positionPath(industry: PlatformResourceTreeNode, department: PlatformResourceTreeNode) {
  return `${industry.name} / ${department.name}`;
}
</script>
