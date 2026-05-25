<template>
  <q-card flat bordered class="agents-filter-card app-entity-toolbar app-entity-toolbar--offset">
    <q-card-section class="app-entity-toolbar__body app-form-field-grid items-end">
      <q-input v-model="keyword" class="app-field-md agent-control" dense outlined clearable debounce="350" placeholder="搜索 Agent...">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="selectedStatus"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="All Types"
        :options="statusOptions"
        class="agent-control"
      />
      <q-select
        v-model="selectedCreator"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="创建者"
        :options="creatorOptions"
        option-value="user_id"
        option-label="label"
        class="agent-control"
      />
      <agent-category-filter v-model="selectedCategory" :tree="categoryTree" />
      <q-select
        v-model="selectedProvider"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="Provider"
        :options="providerOptions"
        class="agent-control"
      />
      <div class="app-actions-bar app-actions-bar--start">
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
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
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
