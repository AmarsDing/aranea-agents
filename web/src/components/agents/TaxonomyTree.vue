<template>
  <q-card flat bordered class="app-entity-glass-panel taxonomy-tree-panel">
    <q-card-section class="taxonomy-tree-panel__body">
      <draggable
        :list="draggableTree"
        item-key="id"
        class="taxonomy-tree-list"
        ghost-class="taxonomy-tree-node--ghost"
        chosen-class="taxonomy-tree-node--chosen"
        :animation="200"
        :delay="100"
        @change="onIndustryReorder"
      >
        <template #item="{ element: industry }">
          <q-expansion-item
            :model-value="isExpanded(industry.id)"
            expand-icon="keyboard_arrow_down"
            class="taxonomy-tree-industry"
            @update:model-value="setExpanded(industry.id, $event)"
          >
            <template #header>
              <taxonomy-tree-node-header
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
              <draggable
                :list="draggableDepartments[industry.id]"
                item-key="id"
                class="taxonomy-tree-department-list"
                ghost-class="taxonomy-tree-node--ghost"
                chosen-class="taxonomy-tree-node--chosen"
                :animation="200"
                :delay="100"
                @change="onDepartmentReorder(industry.id)"
              >
                <template #item="{ element: department }">
                  <q-expansion-item
                    :model-value="isExpanded(department.id)"
                    expand-icon="keyboard_arrow_down"
                    class="taxonomy-tree-department"
                    @update:model-value="setExpanded(department.id, $event)"
                  >
                    <template #header>
                      <taxonomy-tree-node-header
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

                    <draggable
                      :list="draggablePositions[department.id]"
                      item-key="id"
                      class="position-card-grid"
                      ghost-class="position-card--ghost"
                      chosen-class="position-card--chosen"
                      :animation="200"
                      :delay="100"
                      @change="onPositionReorder(department.id)"
                    >
                      <template #item="{ element: position }">
                        <taxonomy-position-card
                          :position="position"
                          :path="positionPath(industry, department)"
                          :readonly="readonly"
                          :highlight="positionHighlighted(position)"
                          @edit="$emit('edit', $event)"
                          @remove="$emit('remove', $event)"
                        />
                      </template>
                    </draggable>

                    <button
                      v-if="!readonly"
                      type="button"
                      class="position-card-add"
                      @click="$emit('create-child', 'position', department)"
                    >
                      <q-icon name="add" size="22px" color="primary" />
                      <span>新增职位</span>
                    </button>
                  </q-expansion-item>
                </template>
              </draggable>

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
        </template>
      </draggable>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import draggable from "vuedraggable";
import TaxonomyPositionCard from "./TaxonomyPositionCard.vue";
import TaxonomyTreeNodeHeader from "./TaxonomyTreeNodeHeader.vue";
import type { PlatformResourceTreeNode } from "../../features/platform/types";
import {
  collectDefaultExpandedIds,
  collectExpandedIdsForFilter,
  departmentPositions,
  nodeMatchesKeyword,
  type TaxonomyLevel
} from "../../features/platform/taxonomyTreeUtils";

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

const emit = defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  "create-child": [level: TaxonomyLevel, parent: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
  "toggle-enabled": [node: PlatformResourceTreeNode, enabled: boolean];
  reorder: [ids: string[]];
}>();

const expandedIds = ref<Set<string>>(new Set());

const draggableTree = reactive<PlatformResourceTreeNode[]>([]);
const draggableDepartments = reactive<Record<string, PlatformResourceTreeNode[]>>({});
const draggablePositions = reactive<Record<string, PlatformResourceTreeNode[]>>({});

watch(
  () => props.tree,
  (tree) => {
    draggableTree.splice(0, draggableTree.length, ...tree);
    for (const industry of tree) {
      const depts = (industry.children ?? []).filter((n) => n.level === "department");
      if (!draggableDepartments[industry.id]) {
        draggableDepartments[industry.id] = [];
      }
      draggableDepartments[industry.id].splice(0, draggableDepartments[industry.id].length, ...depts);
      for (const dept of depts) {
        const positions = departmentPositions(dept);
        if (!draggablePositions[dept.id]) {
          draggablePositions[dept.id] = [];
        }
        draggablePositions[dept.id].splice(0, draggablePositions[dept.id].length, ...positions);
      }
    }
  },
  { immediate: true, deep: false }
);

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

function positionPath(industry: PlatformResourceTreeNode, department: PlatformResourceTreeNode) {
  return `${industry.name} / ${department.name}`;
}

function positionHighlighted(position: PlatformResourceTreeNode) {
  return nodeMatchesKeyword(position, props.keyword);
}

function onIndustryReorder() {
  emit("reorder", draggableTree.map((n) => n.id));
}

function onDepartmentReorder(_industryId: string) {
  const depts = draggableDepartments[_industryId];
  if (depts) {
    emit("reorder", depts.map((n) => n.id));
  }
}

function onPositionReorder(_departmentId: string) {
  const positions = draggablePositions[_departmentId];
  if (positions) {
    emit("reorder", positions.map((n) => n.id));
  }
}
</script>
