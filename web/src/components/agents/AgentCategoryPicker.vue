<template>
  <q-field
    :model-value="displayLabel"
    class="agent-category-picker agent-dialog-control"
    dense
    outlined
    stack-label
    :label="label"
    :disable="disable"
    :clearable="clearable"
    @clear="clearSelection"
  >
    <template #control>
      <div
        class="agent-category-picker__hit row items-center full-width cursor-pointer"
        :class="{ 'is-placeholder': !displayLabel }"
        @click="menuOpen = true"
      >
        <span class="col ellipsis">{{ displayLabel || placeholder }}</span>
        <q-icon name="account_tree" size="18px" color="primary" />
      </div>
    </template>

    <q-menu v-model="menuOpen" anchor="bottom left" self="top left" fit :offset="[0, 6]" class="agent-category-picker-menu">
      <q-card flat class="agent-category-picker-menu__card">
        <q-card-section class="q-pb-sm">
          <q-input
            v-model="menuKeyword"
            dense
            outlined
            clearable
            debounce="150"
            placeholder="搜索行业、部门或职位..."
            class="category-control"
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </q-card-section>
        <q-separator />
        <q-scroll-area class="agent-category-picker-menu__scroll">
          <div class="q-pa-sm">
            <q-tree
              :nodes="menuNodes"
              node-key="id"
              :expanded="expanded"
              dense
              no-connectors
              @update:expanded="onExpandedUpdate"
            >
              <template #default-header="prop">
                <div
                  class="app-category-tree-node row items-center no-wrap full-width"
                  :class="{
                    'app-category-tree-node--selectable': prop.node.selectable,
                    'app-category-tree-node--selected': modelValue === prop.node.id
                  }"
                  @click.stop="onPick(prop.node)"
                >
                  <q-icon :name="prop.node.icon" color="primary" size="16px" class="q-mr-sm" />
                  <div class="col min-width-0">
                    <div class="ellipsis">{{ prop.node.label }}</div>
                    <div v-if="prop.node.caption" class="app-category-tree-node__caption ellipsis">{{ prop.node.caption }}</div>
                  </div>
                  <q-icon v-if="modelValue === prop.node.id" name="check_circle" color="primary" size="18px" />
                </div>
              </template>
            </q-tree>
            <div v-if="menuNodes.length === 0" class="text-caption text-grey-7 q-pa-md text-center">暂无匹配分类</div>
          </div>
        </q-scroll-area>
      </q-card>
    </q-menu>
  </q-field>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { PlatformResourceTreeNode } from "../../features/platform/types";
import {
  collectExpandedIdsForFilter,
  filterCategoryTree,
  findCategoryPath,
  formatCategoryPath,
  toQTreeNodes,
  type CategoryQTreeNode
} from "../../features/platform/categoryTreeUtils";

const props = withDefaults(
  defineProps<{
    modelValue: string | null;
    tree: PlatformResourceTreeNode[];
    label?: string;
    placeholder?: string;
    disable?: boolean;
    clearable?: boolean;
  }>(),
  {
    label: "业务分类",
    placeholder: "选择行业 / 部门 / 职位",
    disable: false,
    clearable: true
  }
);

const emit = defineEmits<{
  "update:modelValue": [value: string | null];
}>();

const menuOpen = ref(false);
const menuKeyword = ref("");
const expanded = ref<string[]>([]);

const filteredTree = computed(() => filterCategoryTree(props.tree.filter((node) => node.level === "industry"), menuKeyword.value));

const menuNodes = computed(() => toQTreeNodes(filteredTree.value, { selectableLevel: "position", enabledOnly: true }));

const displayLabel = computed(() => {
  if (!props.modelValue) return "";
  const path = findCategoryPath(props.tree, props.modelValue);
  return path.length ? formatCategoryPath(path) : "";
});

watch([filteredTree, menuKeyword], () => {
  const fromSearch = collectExpandedIdsForFilter(filteredTree.value, menuKeyword.value);
  expanded.value = fromSearch.length ? fromSearch : filteredTree.value.map((node) => node.id);
});

watch(menuOpen, (open) => {
  if (!open) {
    menuKeyword.value = "";
    return;
  }
  expanded.value = filteredTree.value.map((node) => node.id);
});

function onPick(node: CategoryQTreeNode) {
  if (!node.selectable) return;
  emit("update:modelValue", node.id);
  menuOpen.value = false;
}

function clearSelection() {
  emit("update:modelValue", null);
}

function onExpandedUpdate(value: readonly string[]) {
  expanded.value = [...value];
}
</script>
