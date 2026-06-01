<template>
  <q-card flat bordered class="app-entity-glass-panel category-tree-panel">
    <q-card-section class="category-tree-panel__body">
      <q-list class="category-tree-list">
        <q-expansion-item
          v-for="industry in tree"
          :key="industry.id"
          :model-value="isExpanded(industry.id)"
          expand-icon="keyboard_arrow_down"
          class="category-tree-industry"
          @update:model-value="setExpanded(industry.id, $event)"
        >
          <template #header>
            <category-tree-node-header
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

          <div class="category-tree-industry__body">
            <q-expansion-item
              v-for="department in departmentNodes(industry)"
              :key="department.id"
              :model-value="isExpanded(department.id)"
              expand-icon="keyboard_arrow_down"
              class="category-tree-department"
              @update:model-value="setExpanded(department.id, $event)"
            >
              <template #header>
                <category-tree-node-header
                  :node="department"
                  :show-system-chip="showSystemChip"
                  :readonly="readonly"
                  :toggle-loading="isToggling(department.id)"
                  @edit="$emit('edit', department)"
                  @create-child="$emit('create-child', 'position', department)"
                  @remove="$emit('remove', department)"
                  @toggle-enabled="$emit('toggle-enabled', department, $event)"
                />
              </template>

              <div class="position-card-grid">
                <agent-category-position-card
                  v-for="position in positionNodes(department)"
                  :key="position.id"
                  :position="position"
                  :path="positionPath(industry, department)"
                  :readonly="readonly"
                  :highlight="positionHighlighted(position)"
                  @edit="$emit('edit', $event)"
                  @remove="$emit('remove', $event)"
                />

                <button
                  v-if="!readonly"
                  type="button"
                  class="position-card-add"
                  @click="$emit('create-child', 'position', department)"
                >
                  <q-icon name="add" size="22px" color="primary" />
                  <span>新增职位</span>
                </button>
              </div>
            </q-expansion-item>

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
import { ref, watch } from "vue";
import AgentCategoryPositionCard from "./AgentCategoryPositionCard.vue";
import CategoryTreeNodeHeader from "./CategoryTreeNodeHeader.vue";
import type { PlatformResourceTreeNode } from "../../features/platform/types";
import {
  collectDefaultExpandedIds,
  collectExpandedIdsForFilter,
  departmentPositions,
  nodeMatchesKeyword,
  type CategoryLevel
} from "../../features/platform/categoryTreeUtils";

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
    keyword: "",
    readonly: false,
    showSystemChip: true,
    defaultExpandAll: true,
    togglingIds: () => new Set<string>()
  }
);

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  "create-child": [level: CategoryLevel, parent: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
  "toggle-enabled": [node: PlatformResourceTreeNode, enabled: boolean];
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
  }
);

watch(
  () => props.tree,
  (tree) => {
    if (!props.defaultExpandAll || !tree.length) return;
    if (expandedIds.value.size > 0) return;
    expandedIds.value = collectDefaultExpandedIds(tree);
  },
  { immediate: true, deep: false }
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
  return (industry.children ?? []).filter((node) => node.level === "department");
}

function positionNodes(department: PlatformResourceTreeNode) {
  return departmentPositions(department);
}

function positionPath(industry: PlatformResourceTreeNode, department: PlatformResourceTreeNode) {
  return `${industry.name} / ${department.name}`;
}

function positionHighlighted(position: PlatformResourceTreeNode) {
  return nodeMatchesKeyword(position, props.keyword);
}
</script>
