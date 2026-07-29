<template>
  <q-page :class="['app-standard-page app-entity-page agents-page', { 'is-dark': isDark }]">
    <agents-workspace-hero @create="openCreate" />

    <agents-filters-card
      v-model:keyword="keyword"
      v-model:selected-status="selectedStatus"
      v-model:selected-taxonomy="selectedTaxonomy"
      v-model:selected-creator="selectedCreator"
      v-model:selected-provider="selectedProvider"
      v-model:view-mode="viewMode"
      :status-options="statusOptions"
      :taxonomy-tree="taxonomyTree"
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
      :get-category-label="taxonomyLabel"
      @create="openCreate"
      @toggle-favorite="toggleFavorite"
      @copy-key="copyAgentKey"
      @delete="confirmDelete"
      @duplicate="duplicateListedAgent"
      @reorder="onReorder"
    />

    <agents-pagination-bar v-model:page="page" v-model:rows-per-page="rowsPerPage" :total="total" :page-max="pageMax" />

    <agent-create-dialog
      v-model="createOpen"
      v-model:self-evolve="selfEvolve"
      v-model:agent-kind="agentKind"
      v-model:form="form"
      v-model:a2a-proxy="a2aProxy"
      :is-a2-a-proxy="isA2AProxyCreate"
      :taxonomy-tree="taxonomyTree"
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
      :create-disabled-hint="createDisabledHint"
      :creating="creating"
      :checking-model="checkingModel"
      @apply-template="applyTemplate"
      @check-model="checkModel"
      @create="onCreate"
    />

    <q-dialog v-model="deleteOpen">
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section>
          <div class="text-h6">删除 Agent</div>
          <div class="text-body2 text-grey-7 q-mt-sm">
            确认删除「{{ deleteTarget?.display_name }}」？此操作会软删除，列表中不再显示。
          </div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn v-close-popup flat rounded no-caps label="取消" />
          <q-btn color="negative" rounded unelevated no-caps label="删除" @click="deleteAgentTarget" />
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
import AgentCreateDialog from '../components/agents/AgentCreateDialog.vue';
import AgentsFiltersCard from '../components/agents/AgentsFiltersCard.vue';
import AgentsListSection from '../components/agents/AgentsListSection.vue';
import AgentsPaginationBar from '../components/agents/AgentsPaginationBar.vue';
import AgentsWorkspaceHero from '../components/agents/AgentsWorkspaceHero.vue';
import { useAgentsPage } from '../features/agents/useAgentsPage';

const {
  isDark,
  agents,
  keyword,
  selectedStatus,
  selectedProvider,
  selectedTaxonomy,
  selectedCreator,
  creatorOptions,
  page,
  rowsPerPage,
  total,
  loading,
  createOpen,
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
  taxonomyTree,
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
  createDisabledHint,
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
  taxonomyLabel,
  onReorder,
} = useAgentsPage();
</script>
