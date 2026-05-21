<template>
  <q-card flat bordered class="agents-filter-card">
    <q-card-section class="agents-filter-card__body app-form-field-grid items-end">
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
      <q-select
        v-model="selectedCategory"
        dense
        outlined
        clearable
        emit-value
        map-options
        use-input
        label="业务分类"
        :options="categoryPositionOptions"
        class="agent-control"
      />
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
          class="view-toggle"
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
  categoryPositionOptions: SelectOption[];
  providerOptions: SelectOption[];
  creatorOptions: { user_id: string; label: string }[];
}>();
</script>

<style scoped>
.agents-filter-card {
  margin-top: 22px;
  border: 1px solid var(--glass-border);
  border-radius: 24px;
  background: var(--glass-surface);
  box-shadow: none;
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.agents-filter-card__body {
  padding: 14px 16px;
}

.agent-control :deep(.q-field__control) {
  border-radius: 16px;
  background: var(--glass-elevated);
  min-height: 44px;
}

.agent-control :deep(.q-field__control::before) {
  border-color: var(--glass-border);
}

.agent-control :deep(.q-field__control::after) {
  border-width: 1px;
}

.view-toggle {
  padding: 3px;
  border: 1px solid var(--glass-border);
  border-radius: 999px;
  background: var(--glass-surface);
}
</style>
