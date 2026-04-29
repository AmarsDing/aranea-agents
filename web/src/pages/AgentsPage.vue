<template>
  <q-page :class="['agents-page', { 'is-dark': isDark }]">
    <agents-workspace-hero @create="openCreate" @open-migration="migrationOpen = true" />

    <agents-filters-card
      v-model:keyword="keyword"
      v-model:selected-status="selectedStatus"
      v-model:selected-category="selectedCategory"
      v-model:selected-provider="selectedProvider"
      v-model:view-mode="viewMode"
      :status-options="statusOptions"
      :category-position-options="categoryPositionOptions"
      :provider-options="providerOptions"
    />

    <agents-list-section
      :loading="loading"
      :agents="agents"
      :keyword="keyword"
      :view-mode="viewMode"
      :rows-per-page="rowsPerPage"
      :table-columns="tableColumns"
      :is-favorite="isFavorite"
      :get-category-label="categoryLabel"
      @create="openCreate"
      @toggle-favorite="toggleFavorite"
      @copy-key="copyAgentKey"
      @delete="confirmDelete"
    />

    <agents-pagination-bar v-model:page="page" v-model:rows-per-page="rowsPerPage" :total="total" :page-max="pageMax" />

    <agent-create-dialog
      v-model="createOpen"
      v-model:self-evolve="selfEvolve"
      v-model:category-industry="categoryIndustry"
      v-model:category-department="categoryDepartment"
      :form="form"
      :industry-options="industryOptions"
      :department-options="departmentOptions"
      :position-options="positionOptions"
      :provider-options="providerOptions"
      :model-options="modelOptions"
      :selected-template-key="selectedTemplateKey"
      :agent-key-error="agentKeyError"
      :can-create="canCreate"
      :creating="creating"
      :checking-model="checkingModel"
      @apply-template="applyTemplate"
      @check-model="checkModel"
      @create="onCreate"
    />

    <q-dialog v-model="deleteOpen">
      <q-card style="width: 420px; max-width: 92vw">
        <q-card-section>
          <div class="text-h6">删除 Agent</div>
          <div class="text-body2 text-grey-7 q-mt-sm">确认删除「{{ deleteTarget?.display_name }}」？此操作会软删除，列表中不再显示。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="negative" rounded unelevated label="删除" @click="deleteAgentTarget" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="migrationOpen">
      <q-card style="width: 520px; max-width: 92vw">
        <q-card-section>
          <div class="text-h6">Agent 迁移</div>
          <div class="text-body2 text-grey-7 q-mt-sm">导入、导出、批量映射与冲突处理将在单独流程中实现；当前先保留入口。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn color="primary" flat rounded label="知道了" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
/**
 * 路由容器：组装列表视图子组件，调度用例（由 useAgentsPage 提供）。
 * 表现与交互块在 components/agents/*；领域胶水在 features/agents/useAgentsPage.ts。
 */
import AgentCreateDialog from "../components/agents/AgentCreateDialog.vue";
import AgentsFiltersCard from "../components/agents/AgentsFiltersCard.vue";
import AgentsListSection from "../components/agents/AgentsListSection.vue";
import AgentsPaginationBar from "../components/agents/AgentsPaginationBar.vue";
import AgentsWorkspaceHero from "../components/agents/AgentsWorkspaceHero.vue";
import { useAgentsPage } from "../features/agents/useAgentsPage";

const {
  isDark,
  agents,
  keyword,
  selectedStatus,
  selectedProvider,
  selectedCategory,
  page,
  rowsPerPage,
  total,
  loading,
  createOpen,
  migrationOpen,
  deleteOpen,
  deleteTarget,
  creating,
  checkingModel,
  selfEvolve,
  viewMode,
  categoryIndustry,
  categoryDepartment,
  form,
  selectedTemplateKey,
  providerOptions,
  modelOptions,
  industryOptions,
  departmentOptions,
  positionOptions,
  categoryPositionOptions,
  pageMax,
  tableColumns,
  agentKeyError,
  canCreate,
  statusOptions,
  checkModel,
  onCreate,
  applyTemplate,
  confirmDelete,
  deleteAgentTarget,
  isFavorite,
  toggleFavorite,
  copyAgentKey,
  openCreate,
  categoryLabel
} = useAgentsPage();
</script>

<style scoped>
.agents-page {
  min-height: 100%;
  padding: 28px;
  background: var(--canvas-base);
  color: var(--color-text-primary);
}

.agents-page.is-dark {
  background: var(--canvas-base);
  color: var(--color-text-primary);
}

/* 子组件为 scoped，页级暗色主题用 :deep 命中内部类名 */
.agents-page.is-dark :deep(.agents-kicker) {
  border-color: var(--glass-border-hover, var(--glass-border));
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  color: var(--color-accent);
  box-shadow: none;
}

.agents-page.is-dark :deep(.agents-title) {
  color: var(--color-text-primary);
}

.agents-page.is-dark :deep(.agents-subtitle),
.agents-page.is-dark :deep(.agent-handle) {
  color: var(--color-text-secondary);
}

.agents-page.is-dark :deep(.agents-filter-card),
.agents-page.is-dark :deep(.empty-agent-card),
.agents-page.is-dark :deep(.agents-table),
.agents-page.is-dark :deep(.agents-pagination) {
  border-color: var(--glass-border);
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: none;
}

.agents-page.is-dark :deep(.agent-control .q-field__control),
.agents-page.is-dark :deep(.rows-select .q-field__control) {
  background: var(--glass-surface-hover);
}

.agents-page.is-dark :deep(.agent-control .q-field__control::before),
.agents-page.is-dark :deep(.rows-select .q-field__control::before) {
  border-color: var(--glass-border);
}

.agents-page.is-dark :deep(.view-toggle) {
  border-color: var(--glass-border);
  background: var(--glass-surface);
}

.agents-page.is-dark :deep(.empty-agent-card) {
  background: var(--glass-surface);
}

.agents-page.is-dark :deep(.empty-agent-visual) {
  border-color: var(--glass-border);
  background: var(--glass-surface-hover);
  box-shadow: none;
}

.agents-page.is-dark :deep(.agents-table th) {
  background: var(--glass-surface-hover);
  color: var(--color-text-secondary);
}

.agents-page.is-dark :deep(.agents-table td) {
  color: var(--color-text-primary);
}

.agents-page.is-dark :deep(.agents-table tbody tr:hover) {
  background: var(--glass-surface-hover);
}

@media (max-width: 599px) {
  .agents-page {
    padding: 18px;
  }
}
</style>
