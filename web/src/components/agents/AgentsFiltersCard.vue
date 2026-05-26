<template>
  <AppPageToolbar variant="entity" offset class="agents-filter-card">
    <q-input v-model="keyword" class="app-page-toolbar__search agent-control" dense outlined clearable debounce="350" placeholder="搜索 Agent...">
      <template #prepend><q-icon name="search" /></template>
    </q-input>
    <q-select
      v-model="selectedStatus"
      class="app-page-toolbar__field agent-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="All Types"
      :options="statusOptions"
    />
    <q-select
      v-model="selectedCreator"
      class="app-page-toolbar__field agent-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="创建者"
      :options="creatorOptions"
      option-value="user_id"
      option-label="label"
    />
    <agent-category-filter v-model="selectedCategory" class="app-page-toolbar__field agent-category-field--toolbar" :tree="categoryTree" />
    <q-select
      v-model="selectedProvider"
      class="app-page-toolbar__field agent-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="Provider"
      :options="providerOptions"
    />
    <template #actions>
      <q-btn-toggle
        v-model="viewMode"
        dense
        rounded
        unelevated
        class="app-view-toggle"
        toggle-color="primary"
        :options="[
          { value: 'grid', slot: 'grid' },
          { value: 'list', slot: 'list' }
        ]"
      >
        <template #grid><q-icon name="grid_view" /></template>
        <template #list><q-icon name="view_list" /></template>
      </q-btn-toggle>
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import AppPageToolbar from "../layout/AppPageToolbar.vue";
import AgentCategoryFilter from "./AgentCategoryFilter.vue";
import type { PlatformResourceTreeNode } from "../../features/platform/types";

type SelectOption = { label: string; value: string };

type ViewMode = "grid" | "list";

const keyword = defineModel<string>("keyword", { default: "" });
const selectedStatus = defineModel<string | null>("selectedStatus", { default: null });
const selectedCategory = defineModel<string | null>("selectedCategory", { default: null });
const selectedCreator = defineModel<string | null>("selectedCreator", { default: null });
const selectedProvider = defineModel<string | null>("selectedProvider", { default: null });
const viewMode = defineModel<ViewMode>("viewMode", { default: "grid" });

defineProps<{
  statusOptions: SelectOption[];
  categoryTree: PlatformResourceTreeNode[];
  providerOptions: SelectOption[];
  creatorOptions: { user_id: string; label: string }[];
}>();
</script>
