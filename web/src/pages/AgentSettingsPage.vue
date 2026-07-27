<template>
  <q-page :class="['agent-settings', { 'agent-settings--files-fill': tab === 'files' }]">
    <q-card flat bordered :class="['settings-shell', { 'settings-shell--fill': tab === 'files' }]">
      <agent-settings-header
        :agent="form"
        :show-evolving="showEvolving"
        :favorite="form.is_favorite"
        :saving="saving"
        @back="goBack"
        @change-avatar="avatarPickerOpen = true"
        @open-prompt="promptDialog = true"
        @open-advanced="advancedDialog = true"
        @toggle-favorite="toggleFavorite"
        @save="saveAgent"
      />
      <q-separator />

      <q-banner v-if="loadError" rounded class="bg-negative text-white q-ma-md">
        <template #avatar>
          <q-icon name="error" />
        </template>
        {{ loadError }}
        <template #action>
          <q-btn flat color="white" label="重试" @click="loadInitial" />
        </template>
      </q-banner>

      <template v-if="!loadError">
        <q-tabs v-model="tab" dense align="left" class="agent-settings-tabs" :breakpoint="0">
          <q-tab name="agent" label="Agent 属性" />
          <q-tab name="memory" label="记忆" />
          <q-tab name="files" label="文件" />
          <q-tab name="permissions" label="权限" />
          <q-tab name="skills" label="Skill / 工具" />
          <q-tab name="evolution" label="进化" />
          <q-tab name="learning" label="学习闭环" />
          <q-tab name="hooks" label="钩子" />
          <q-tab name="a2a" label="A2A 协议" />
        </q-tabs>
        <q-separator />

        <q-tab-panels v-model="tab" animated class="settings-panels">
          <q-tab-panel name="agent">
            <agent-settings-agent-tab
              v-model:planner-form="plannerForm"
              v-model:ralph-loop-form="ralphLoopForm"
              v-model:selected-provider-model-id="selectedProviderModelIDModel"
              v-model:form="form"
              v-model:config="config"
              :agent-id="agentId"
              :taxonomy-tree="taxonomyTree"
              :prompt-modes="promptModes"
              :status-options="statusOptions"
              :filtered-provider-model-options="filteredProviderModelOptions"
              :loading-provider-models="loadingProviderModels"
              :orphan-provider-model="orphanProviderModel"
              :disabled-catalog-match="disabledCatalogMatch"
              :checking-agent-model="checkingAgentModel"
              :agent-model-check-ok="agentModelCheckOk"
              :agent-model-check-message="agentModelCheckMessage"
              @copy-key="copyKey"
              @open-permissions-tab="tab = 'permissions'"
              @open-memory-tab="tab = 'memory'"
              @open-provider-manager="openProviderManager"
              @filter-provider-models="filterProviderModels"
              @reset-provider-model-filter="resetProviderModelFilter"
              @refine-error="onRefineError"
            />
          </q-tab-panel>

          <q-tab-panel name="memory">
            <agent-settings-memory-tab
              v-model:config="config"
              :truncate-strategy-options="truncateStrategyOptions"
              :snapshot-mode-options="snapshotModeOptions"
              :memory-scope-options="memoryScopeOptions"
              :pii-policy-options="piiPolicyOptions"
              :available-optional-files="availableOptionalFiles"
              @add-optional-file="addOptionalFile"
              @open-evolution-tab="tab = 'evolution'"
            />
          </q-tab-panel>
          <q-tab-panel name="files" class="settings-tab-panel-fill">
            <agent-files-panel
              v-model:active-file="activeFile"
              v-model:splitter="fileSplitter"
              :files="files"
              :file-token-by-name="fileTokenByName"
              :dirty="fileDirty"
              :agent-id="toValue(agentId)"
              :refine-fn="refinePromptField"
              @update-file-body="updateFileBody"
              @confirm-reload="confirmFileReload"
              @reload="reloadActiveFile"
              @save="saveAgent"
              @refine-error="onRefineError"
            />
          </q-tab-panel>

          <q-tab-panel name="permissions">
            <div class="settings-grid settings-grid--wide">
              <agent-usage-quota-panel v-if="form.id" :agent-id="form.id" />
              <q-banner v-else rounded class="settings-placeholder-banner">加载 Agent 后可配置用量配额。</q-banner>
            </div>
          </q-tab-panel>

          <q-tab-panel name="skills">
            <agent-settings-skills-tab
              v-model:config="config"
              :agent-id="toValue(agentId)"
              :skill-slug-options="skillSlugOptions"
              :loading-skill-slugs="loadingSkillSlugs"
              :code-executor-capabilities="codeExecutorCapabilities"
              :tool-profile-options="toolProfileOptions"
              :tool-select-options="toolSelectOptions"
              :loading-catalog-tools="loadingCatalogTools"
              :tool-conflicts="toolConflicts"
              @load-skill-slugs="loadSkillSlugOptions"
              @reset-skill-defaults="confirmResetSkillDefaults"
            />
          </q-tab-panel>
          <q-tab-panel name="evolution">
            <agent-evolution-panel
              v-model:evolution="config.evolution"
              v-model:evolution-settings="config.evolutionSettings"
              v-model:guardrails="config.evolution_guardrails"
              :agent-id="agentId"
            />
          </q-tab-panel>

          <q-tab-panel name="learning">
            <agent-learning-loop-panel :agent-id="agentId" />
          </q-tab-panel>

          <q-tab-panel name="hooks">
            <div class="settings-grid settings-grid--wide">
              <section class="settings-section">
                <agent-hooks-panel :agent-id="agentId" :agent-key="form.agent_key" />
              </section>
            </div>
          </q-tab-panel>

          <q-tab-panel name="a2a">
            <agent-settings-a2-a-tab
              v-if="form.agent_kind === 'a2a_proxy'"
              :agent-id="agentId"
              :a2a-proxy="form.a2a_proxy_config"
              @saved="reloadAgent"
            />
            <agent-settings-a2-a-endpoint-tab v-else :agent-id="agentId" />
          </q-tab-panel>
        </q-tab-panels>
      </template>
      <q-inner-loading :showing="pageLoading" />
    </q-card>

    <agent-prompt-preview-dialog
      v-model:open="promptDialog"
      v-model:mode="previewMode"
      :modes="promptModes"
      :instruction-text="promptInstructionText"
      :static-tokens="promptStaticTokens"
      :runtime-tokens="promptRuntimeTokens"
      :sections="promptPreviewSections"
    />

    <agent-advanced-dialog
      v-model:open="advancedDialog"
      :saving="saving"
      :channel-options="advancedChannelOptions"
      :loading-channels="loadingAdvancedChannels"
      :channel-id="advancedState.channel_id"
      :chat-id-input="advancedState.chat_id"
      :workspace-input="advancedState.workspace"
      :reasoning-mode-input="advancedState.reasoning_mode"
      :reasoning-level-input="advancedState.reasoning_level"
      :compaction-enabled-input="advancedState.context_compaction_enabled"
      :session-summary-enabled-input="advancedState.session_summary_enabled"
      @save="onAdvancedSave"
    />
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, toValue } from 'vue';
import AgentAvatarPicker from '../components/avatar/AgentAvatarPicker.vue';
import AgentEvolutionPanel from '../components/agents/AgentEvolutionPanel.vue';
import AgentLearningLoopPanel from '../components/agents/AgentLearningLoopPanel.vue';
import AgentFilesPanel from '../components/agents/AgentFilesPanel.vue';
import { refinePromptField } from '../features/agents/aiRefine';
import AgentSettingsHeader from '../components/agents/AgentSettingsHeader.vue';
import AgentAdvancedDialog from '../components/agents/AgentAdvancedDialog.vue';
import AgentHooksPanel from '../components/agents/AgentHooksPanel.vue';
import AgentPromptPreviewDialog from '../components/agents/AgentPromptPreviewDialog.vue';
import AgentSettingsAgentTab from './agent-settings/AgentSettingsAgentTab.vue';
import AgentSettingsMemoryTab from './agent-settings/AgentSettingsMemoryTab.vue';
import AgentSettingsSkillsTab from './agent-settings/AgentSettingsSkillsTab.vue';
import AgentSettingsA2ATab from '../components/agents/AgentSettingsA2ATab.vue';
import AgentSettingsA2AEndpointTab from '../components/agents/AgentSettingsA2AEndpointTab.vue';
import AgentUsageQuotaPanel from '../components/agents/AgentUsageQuotaPanel.vue';
import { useAgentSettingsPage } from '../features/agents/useAgentSettingsPage';

const {
  tab,
  form,
  config,
  plannerForm,
  ralphLoopForm,
  saving,
  router,
  taxonomyTree,
  avatarPickerOpen,
  promptDialog,
  advancedDialog,
  loadError,
  pageLoading,
  toggleFavorite,
  reloadAgent,
  loadInitial,
  saveAgent,
  confirmFileReload,
  promptModes,
  statusOptions,
  copyKey,
  selectedProviderModelIDModel,
  filteredProviderModelOptions,
  loadingProviderModels,
  orphanProviderModel,
  disabledCatalogMatch,
  checkingAgentModel,
  agentModelCheckOk,
  agentModelCheckMessage,
  openProviderManager,
  filterProviderModels,
  resetProviderModelFilter,
  toolProfileOptions,
  toolSelectOptions,
  loadingCatalogTools,
  toolConflicts,
  agentId,
  availableOptionalFiles,
  addOptionalFile,
  onRefineError,
  activeFile,
  fileSplitter,
  files,
  fileDirty,
  updateFileBody,
  reloadActiveFile,
  truncateStrategyOptions,
  snapshotModeOptions,
  memoryScopeOptions,
  piiPolicyOptions,
  loadSkillSlugOptions,
  confirmResetSkillDefaults,
  loadingSkillSlugs,
  skillSlugOptions,
  codeExecutorCapabilities,
  showEvolving,
  fileTokenByName,
  tokenEstimateFor,
  previewMode,
  promptPreview,
  advancedState,
  onAdvancedSave,
  advancedChannelOptions,
  loadingAdvancedChannels,
} = useAgentSettingsPage();

/** Return to the previous page (e.g. chat); fall back to agents list. */
function goBack() {
  if (window.history.length > 1) {
    router.back();
  } else {
    router.push({ name: 'agents' });
  }
}

const promptInstructionText = computed(() => {
  const p = toValue(promptPreview);
  const instruction = String(p.instruction ?? '').trim();
  if (instruction) return instruction;
  const summary = String(p.summary ?? '').trim();
  return summary || '（当前模式下无 System Prompt 内容）';
});

const promptStaticTokens = computed(() => {
  const p = toValue(promptPreview);
  if (p.static_total_tokens > 0) return p.static_total_tokens;
  return tokenEstimateFor(promptInstructionText.value);
});

const promptRuntimeTokens = computed(() => {
  const p = toValue(promptPreview);
  if (p.runtime_overlay_est_tokens > 0) return p.runtime_overlay_est_tokens;
  return p.sections
    .filter((row) => row.source === 'runtime' && row.est_tokens > 0)
    .reduce((sum, row) => sum + row.est_tokens, 0);
});

const promptPreviewSections = computed(() => toValue(promptPreview).sections);
</script>
