<template>
  <q-page :class="['agent-settings', { 'agent-settings--files-fill': tab === 'files' }]">
    <q-card flat bordered :class="['settings-shell', { 'settings-shell--fill': tab === 'files' }]">
      <agent-settings-header
        :agent="form"
        :self-evolve="config.self_evolve"
        :show-evolving="showEvolving"
        :favorite="form.is_favorite"
        :saving="saving"
        @back="router.back()"
        @change-avatar="avatarPickerOpen = true"
        @open-prompt="promptDialog = true"
        @open-advanced="advancedDialog = true"
        @toggle-favorite="toggleFavorite"
        @save="saveAgent"
      />
      <q-separator />
      <q-tabs v-model="tab" dense align="left" class="agent-settings-tabs" :breakpoint="0">
        <q-tab name="agent" label="Agent" />
        <q-tab name="memory" label="记忆" />
        <q-tab name="files" label="文件" />
        <q-tab name="permissions" label="权限" />
        <q-tab name="skills" label="Skill" />
        <q-tab name="evolution" label="进化" />
        <q-tab name="hooks" label="钩子" />
        <q-tab name="a2a" label="A2A" />
        <q-tab name="instances" label="用户实例" />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="settings-panels">
        <q-tab-panel name="agent">
          <agent-settings-agent-tab
            :form="form"
            v-model:planner-form="plannerForm"
            v-model:ralph-loop-form="ralphLoopForm"
            :config="config"
            :agent-id="toValue(agentId)"
            :prompt-modes="promptModes"
            :status-options="statusOptions"
            :selected-provider-model-id="toValue(selectedProviderModelID)"
            :filtered-provider-model-options="toValue(filteredProviderModelOptions)"
            :loading-provider-models="toValue(loadingProviderModels)"
            :tool-profile-options="toolProfileOptions"
            :tool-select-options="toolSelectOptions"
            :loading-catalog-tools="loadingCatalogTools"
            :tool-conflicts="toolConflicts"
            :heartbeat-file="heartbeatFile"
            @copy-key="copyKey"
            @open-permissions-tab="tab = 'permissions'"
            @filter-provider-models="filterProviderModels"
            @select-provider-model="selectProviderModel"
          />
        </q-tab-panel>

        <q-tab-panel name="memory">
          <agent-settings-memory-tab
            :config="config"
            :truncate-strategy-options="truncateStrategyOptions"
            :snapshot-mode-options="snapshotModeOptions"
            :memory-scope-options="memoryScopeOptions"
          />
        </q-tab-panel>
        <q-tab-panel name="files" class="settings-tab-panel-fill">
          <agent-files-panel
            v-model:active-file="activeFile"
            v-model:splitter="fileSplitter"
            :files="files"
            :file-token-by-name="fileTokenByName"
            :dirty="fileDirty"
            @update-file-body="updateFileBody"
            @reload="reloadActiveFile"
            @ai-edit="aiEditOpen = true"
            @save="saveAgent"
          />
        </q-tab-panel>

        <q-tab-panel name="permissions">
          <div class="settings-grid">
            <agent-usage-quota-panel v-if="form.id" :agent-id="form.id" />
            <q-banner v-else rounded class="settings-placeholder-banner">加载 Agent 后可配置用量配额。</q-banner>
          </div>
        </q-tab-panel>

        <q-tab-panel name="skills">
          <agent-settings-skills-tab
            :config="config"
            :skill-slug-options="skillSlugOptions"
            :loading-skill-slugs="loadingSkillSlugs"
            :code-executor-capabilities="codeExecutorCapabilities"
            @load-skill-slugs="loadSkillSlugOptions"
            @reset-skill-defaults="resetSkillRuntimeDefaults"
          />
        </q-tab-panel>
        <q-tab-panel name="evolution">
          <agent-evolution-panel v-model:range="evolutionRange" :agent-id="agentId" :evolution="config.evolution" :guardrails="config.evolution_guardrails" />
        </q-tab-panel>

        <q-tab-panel name="hooks">
          <agent-hooks-panel :agent-id="agentId" :agent-key="form.agent_key" />
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

        <q-tab-panel name="instances">
          <q-banner rounded class="settings-placeholder-banner">用户实例用于按用户覆盖 USER.md、权限与默认上下文；当前保留入口。</q-banner>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>

    <q-dialog v-model="promptDialog">
      <q-card class="prompt-dialog app-dialog-card">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">系统提示词</div>
            <div class="text-caption text-grey-7">{{ tokenEstimateFor(promptPreview) }} tokens</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-tabs v-model="previewMode" dense active-color="primary" indicator-color="primary">
          <q-tab v-for="mode in promptModes" :key="mode.value" :name="mode.value" :label="mode.label" />
        </q-tabs>
        <q-separator />
        <q-card-section>
          <pre class="agent-prompt-preview">{{ promptPreview }}</pre>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="aiEditOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">AI 编辑</div>
            <div class="text-caption text-grey-7">描述您想要更改的内容。AI 将读取当前文件并相应更新。</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-card-section class="app-dialog-body">
          <q-input v-model="aiInstruction" class="app-field-long" outlined type="textarea" rows="6" label="编辑指令" placeholder="例如：使 Agent 更正式、添加中文支持..." />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated no-caps label="重新生成" :loading="aiEditing" @click="applyAiEdit" />
        </q-card-actions>
      </q-card>
    </q-dialog>
    <agent-advanced-dialog
      v-model:open="advancedDialog"
      :saving="saving"
      :channel-id="advancedState.channel_id"
      :chat-id-input="advancedState.chat_id"
      :workspace-input="advancedState.workspace"
      :reasoning-mode-input="advancedState.reasoning_mode"
      :reasoning-level-input="advancedState.reasoning_level"
      :compaction-enabled-input="advancedState.context_compaction_enabled"
      :session-summary-enabled-input="advancedState.session_summary_enabled"
      :truncate-strategy-input="advancedState.truncate_strategy"
      :recent-window-turns-input="advancedState.recent_window_turns"
      :recent-window-tokens-input="advancedState.recent_window_tokens"
      :summary-keep-turns-input="advancedState.summary_keep_turns"
      @save="onAdvancedSave"
    />
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-page>
</template>

<script setup lang="ts">
import { toValue } from "vue";
import AgentAvatarPicker from "../components/avatar/AgentAvatarPicker.vue";
import AgentEvolutionPanel from "../components/agents/AgentEvolutionPanel.vue";
import AgentFilesPanel from "../components/agents/AgentFilesPanel.vue";
import AgentSettingsHeader from "../components/agents/AgentSettingsHeader.vue";
import AgentAdvancedDialog from "../components/agents/AgentAdvancedDialog.vue";
import AgentHooksPanel from "../components/agents/AgentHooksPanel.vue";
import AgentSettingsAgentTab from "./agent-settings/AgentSettingsAgentTab.vue";
import AgentSettingsMemoryTab from "./agent-settings/AgentSettingsMemoryTab.vue";
import AgentSettingsSkillsTab from "./agent-settings/AgentSettingsSkillsTab.vue";
import AgentSettingsA2ATab from "../components/agents/AgentSettingsA2ATab.vue";
import AgentSettingsA2AEndpointTab from "../components/agents/AgentSettingsA2AEndpointTab.vue";
import AgentUsageQuotaPanel from "../components/agents/AgentUsageQuotaPanel.vue";
import { useAgentSettingsPage } from "../features/agents/useAgentSettingsPage";

const {
  tab,
  form,
  config,
  plannerForm,
  ralphLoopForm,
  saving,
  router,
  avatarPickerOpen,
  promptDialog,
  advancedDialog,
  toggleFavorite,
  reloadAgent,
  saveAgent,
  promptModes,
  statusOptions,
  copyKey,
  selectedProviderModelID,
  filteredProviderModelOptions,
  loadingProviderModels,
  filterProviderModels,
  selectProviderModel,
  toolProfileOptions,
  toolSelectOptions,
  loadingCatalogTools,
  toolConflicts,
  agentId,
  heartbeatFile,
  activeFile,
  fileSplitter,
  files,
  fileDirty,
  updateFileBody,
  reloadActiveFile,
  aiEditOpen,
  aiEditing,
  truncateStrategyOptions,
  snapshotModeOptions,
  memoryScopeOptions,
  loadSkillSlugOptions,
  resetSkillRuntimeDefaults,
  loadingSkillSlugs,
  skillSlugOptions,
  codeExecutorCapabilities,
  evolutionRange,
  showEvolving,
  fileTokenByName,
  tokenEstimateFor,
  previewMode,
  promptPreview,
  aiInstruction,
  applyAiEdit,
  advancedState,
  onAdvancedSave
} = useAgentSettingsPage();
</script>

<style scoped lang="scss">
@import "./agent-settings/agent-settings-page.scss";
</style>
