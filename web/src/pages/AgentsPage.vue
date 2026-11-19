<template>
  <q-page :class="['app-standard-page app-entity-page agents-page', { 'is-dark': isDark }]">
    <agents-workspace-hero @create="openCreate" />

    <div class="agents-workspace" :class="{ 'agents-workspace--inspecting': inspectorOpen }">
      <div class="agents-workspace__main min-width-0">
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
      :selected-ids="selectedAgentIds"
      :error="listError"
      :batch-busy="batchBusyAction"
      :batch-error="batchError"
      @create="openCreate"
      @retry="runLoadList"
      @inspect="inspectAgent"
      @toggle-favorite="toggleFavorite"
      @toggle-select="toggleAgentSelected"
      @copy-key="copyAgentKey"
      @delete="confirmDelete"
      @duplicate="duplicateListedAgent"
      @reorder="onReorder"
      @batch-enable="runBatchSetStatus('active')"
      @batch-disable="runBatchSetStatus('inactive')"
      @batch-delete="runBatchDelete"
      @clear-selection="clearAgentSelection"
    />

    <agents-pagination-bar v-model:page="page" v-model:rows-per-page="rowsPerPage" :total="total" :page-max="pageMax" />
      </div>

      <AppRegistryInspector
        v-model="inspectorOpen"
        :title="inspectedAgent?.display_name || t('shell.registry.inspect')"
        :eyebrow="inspectedAgent ? `AGENT / ${inspectedAgent.agent_key}` : undefined"
      >
        <div v-if="inspectedAgent" class="registry-inspector-stack">
          <div class="row items-center q-gutter-sm">
            <AppStatusChip :status="inspectedAgent.status" />
            <span v-if="inspectedAgent.last_run_status" class="app-registry-cell-sub">
              {{ t('shell.registry.lastRun') }} · {{ inspectedAgent.last_run_status }}
            </span>
          </div>
          <p class="registry-inspector-description">{{ inspectedAgent.agent_description || t('shell.registry.noDescription') }}</p>
          <dl class="registry-facts">
            <div><dt>{{ t('shell.registry.identifier') }}</dt><dd class="mono">{{ inspectedAgent.agent_key }}</dd></div>
            <div><dt>{{ t('shell.registry.model') }}</dt><dd>{{ inspectedAgent.provider }} / {{ inspectedAgent.model }}</dd></div>
            <div><dt>{{ t('shell.registry.organization') }}</dt><dd>{{ taxonomyLabel(inspectedAgent.taxonomy_position_id) }}</dd></div>
            <div><dt>{{ t('shell.registry.contextWindow') }}</dt><dd class="mono">{{ inspectedAgent.context_window }}</dd></div>
            <div><dt>{{ t('shell.registry.lastRunAt') }}</dt><dd class="mono">{{ formatInspectorTime(inspectedAgent.last_run_at) }}</dd></div>
            <div><dt>{{ t('shell.registry.updatedAt') }}</dt><dd class="mono">{{ formatInspectorTime(inspectedAgent.updated_at) }}</dd></div>
          </dl>
          <section v-if="inspectedAgent.settings" class="registry-inspector-section">
            <h3>{{ t('shell.registry.relatedSettings') }}</h3>
            <dl class="registry-facts">
              <div><dt>{{ t('shell.registry.tools') }}</dt><dd>{{ inspectedAgent.settings.tools_enabled ? t('shell.registry.enabled') : t('shell.registry.disabled') }}</dd></div>
              <div><dt>{{ t('shell.registry.memory') }}</dt><dd>{{ inspectedAgent.settings.memory_enabled ? t('shell.registry.enabled') : t('shell.registry.disabled') }}</dd></div>
              <div><dt>{{ t('shell.registry.subagents') }}</dt><dd>{{ inspectedAgent.settings.subagents_enabled ? t('shell.registry.enabled') : t('shell.registry.disabled') }}</dd></div>
            </dl>
          </section>
          <q-btn
            outline
            no-caps
            color="primary"
            icon="settings"
            :label="t('shell.registry.openSettings')"
            :to="`/agents/${inspectedAgent.id}/settings`"
          />
        </div>
      </AppRegistryInspector>
    </div>

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
      @create-attempt="onCreateAttempt"
      @create="onCreate"
    />

    <AppConfirmDialog
      v-model="deleteOpen"
      :title="t('shell.agents.deleteTitle')"
      :message="t('shell.agents.deleteMessage', { name: deleteTarget?.display_name ?? '' })"
      :confirm-label="t('shell.agents.delete')"
      @confirm="deleteAgentTarget"
    />
  </q-page>
</template>

<script setup lang="ts">
/**
 * 路由容器：组装列表视图子组件，调度用例（由 useAgentsPage 提供）。
 * 表现与交互块在 components/agents/*；领域胶水在 features/agents/useAgentsPage.ts。
 */
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AgentCreateDialog from '../components/agents/AgentCreateDialog.vue';
import AgentsFiltersCard from '../components/agents/AgentsFiltersCard.vue';
import AgentsListSection from '../components/agents/AgentsListSection.vue';
import AgentsPaginationBar from '../components/agents/AgentsPaginationBar.vue';
import AgentsWorkspaceHero from '../components/agents/AgentsWorkspaceHero.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import AppConfirmDialog from '../components/common/AppConfirmDialog.vue';
import { useAgentsPage } from '../features/agents/useAgentsPage';
import type { Agent } from '../features/agents/types';

const { t } = useI18n();
const inspectorOpen = ref(false);
const inspectedAgent = ref<Agent | null>(null);

function inspectAgent(agent: Agent) {
  inspectedAgent.value = agent;
  inspectorOpen.value = true;
}

function formatInspectorTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

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
  listError,
  runLoadList,
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
  onCreateAttempt,
  applyTemplate,
  confirmDelete,
  deleteAgentTarget,
  isFavorite,
  toggleFavorite,
  copyAgentKey,
  openCreate,
  taxonomyLabel,
  onReorder,
  selectedAgentIds,
  toggleAgentSelected,
  clearAgentSelection,
  batchBusyAction,
  batchError,
  runBatchSetStatus,
  runBatchDelete,
} = useAgentsPage();
</script>
