<template>
  <q-page :class="['app-standard-page app-entity-page agents-page', { 'is-dark': isDark }]">
    <agents-workspace-hero @create="openCreate" @open-migration="migrationOpen = true" />

    <agents-filters-card
      v-model:keyword="keyword"
      v-model:selected-status="selectedStatus"
      v-model:selected-category="selectedCategory"
      v-model:selected-creator="selectedCreator"
      v-model:selected-provider="selectedProvider"
      v-model:view-mode="viewMode"
      :status-options="statusOptions"
      :category-tree="categoryTree"
      :provider-options="providerOptions"
      :creator-options="creatorOptions"
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
      @duplicate="duplicateListedAgent"
    />

    <agents-pagination-bar v-model:page="page" v-model:rows-per-page="rowsPerPage" :total="total" :page-max="pageMax" />

    <agent-create-dialog
      v-model="createOpen"
      v-model:self-evolve="selfEvolve"
      v-model:agent-kind="agentKind"
      :form="form"
      :a2a-proxy="a2aProxy"
      :isA2AProxy="isA2AProxyCreate"
      :category-tree="categoryTree"
      :provider-options="providerOptions"
      :model-options="modelOptions"
      :selected-template-key="selectedTemplateKey"
      :templates="createTemplates"
      :agent-key-error="agentKeyError"
      :display-name-error="displayNameError"
      :provider-model-error="providerModelError"
      :remote-url-error="remoteUrlError"
      :create-form-error="createFormError"
      :can-create="canCreate"
      :creating="creating"
      :checking-model="checkingModel"
      @apply-template="applyTemplate"
      @check-model="checkModel"
      @create="onCreate"
    />

    <q-dialog v-model="deleteOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">删除 Agent</div>
          <div class="text-body2 text-grey-7 q-mt-sm">确认删除「{{ deleteTarget?.display_name }}」？此操作会软删除，列表中不再显示。</div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps label="取消" v-close-popup />
          <q-btn color="negative" rounded unelevated no-caps label="删除" @click="deleteAgentTarget" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="migrationOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">Agent 迁移</div>
          <div class="text-body2 text-grey-7 q-mt-sm">导入、导出、批量映射与冲突处理将在单独流程中实现；当前先保留入口。</div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn color="primary" flat rounded no-caps label="知道了" v-close-popup />
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
  selectedCreator,
  creatorOptions,
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
  agentKind,
  a2aProxy,
  isA2AProxyCreate,
  viewMode,
  form,
  selectedTemplateKey,
  createTemplates,
  duplicateListedAgent,
  categoryTree,
  providerOptions,
  modelOptions,
  pageMax,
  tableColumns,
  agentKeyError,
  displayNameError,
  providerModelError,
  remoteUrlError,
  createFormError,
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
