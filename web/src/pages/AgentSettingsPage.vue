<template>
  <q-page :class="['agent-settings', { 'agent-settings--files-fill': tab === 'files' }]">
    <q-card flat bordered :class="['settings-shell', { 'settings-shell--fill': tab === 'files' }]">
      <agent-settings-header
        :agent="form"
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
        <q-tab name="skills" label="Skill / 工具" />
        <q-tab name="evolution" label="进化" />
        <q-tab name="hooks" label="钩子" />
        <q-tab name="a2a" label="A2A" />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="settings-panels">
        <q-tab-panel name="agent">
          <agent-settings-agent-tab
            :form="form"
            v-model:planner-form="plannerForm"
            v-model:ralph-loop-form="ralphLoopForm"
            v-model:selected-provider-model-id="selectedProviderModelIDModel"
            :config="config"
            :agent-id="agentId"
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
          />
        </q-tab-panel>

        <q-tab-panel name="memory">
          <agent-settings-memory-tab
            :config="config"
            :truncate-strategy-options="truncateStrategyOptions"
            :snapshot-mode-options="snapshotModeOptions"
            :memory-scope-options="memoryScopeOptions"
            :heartbeat-file="heartbeatFile"
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
            :agent-id="toValue(agentId)"
            :skill-slug-options="skillSlugOptions"
            :loading-skill-slugs="loadingSkillSlugs"
            :code-executor-capabilities="codeExecutorCapabilities"
            :tool-profile-options="toolProfileOptions"
            :tool-select-options="toolSelectOptions"
            :loading-catalog-tools="loadingCatalogTools"
            :tool-conflicts="toolConflicts"
            @load-skill-slugs="loadSkillSlugOptions"
            @reset-skill-defaults="resetSkillRuntimeDefaults"
          />
        </q-tab-panel>
        <q-tab-panel name="evolution">
          <agent-evolution-panel
            v-model:range="evolutionRange"
            :agent-id="agentId"
            :evolution="config.evolution"
            :evolution-settings="config.evolutionSettings"
            :guardrails="config.evolution_guardrails"
            :metrics-loading="evolutionMetricsLoading"
            :metrics="evolutionMetrics"
            :suggestions="evolutionSuggestions"
            :applying-id="evolutionApplyingId"
            :rejecting-id="evolutionRejectingId"
            :pending-suggestions-count="evolutionPendingCount"
            @apply="applyEvolutionSuggestion"
            @reject="rejectEvolutionSuggestion"
          />
        </q-tab-panel>

        <q-tab-panel name="hooks">
          <div class="settings-grid">
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
          <agent-settings-a2-a-endpoint-tab
            v-else
            :loading="a2aEndpoint.loading"
            :saving="a2aEndpoint.saving"
            :card="a2aEndpoint.card"
            :capability-lines="a2aEndpoint.capabilityLines"
            @save="a2aEndpoint.saveEndpoint()"
            @update:card-enabled="a2aEndpoint.card && (a2aEndpoint.card.enabled = $event)"
            @update:capability-lines="a2aEndpoint.capabilityLines = $event"
          />
        </q-tab-panel>

      </q-tab-panels>
    </q-card>

    <q-dialog v-model="promptDialog">
      <q-card class="prompt-dialog app-dialog-card">
        <q-card-section class="row items-center justify-between prompt-dialog__header">
          <div>
            <div class="text-h6">系统提示词</div>
            <div class="text-caption prompt-dialog__stats">
              构建期约 {{ promptStaticTokens }} tokens
              · 运行时追加约 {{ promptRuntimeTokens }} tokens
            </div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-tabs v-model="previewMode" dense align="left" narrow-indicator class="prompt-dialog__mode-tabs">
          <q-tab v-for="mode in promptModes" :key="mode.value" :name="mode.value" :label="mode.label" />
        </q-tabs>
        <q-separator />
        <q-card-section class="prompt-dialog__body">
          <p class="prompt-dialog__hint">
            下方为<strong>构建期</strong>写入模型的 System Prompt（Description、Prompt 文件、运行时策略）。
            实际对话时还会按开关追加记忆、Skills、Intent 等，可在「Token 分解」中查看估算。
          </p>
          <pre class="agent-prompt-preview">{{ promptInstructionText }}</pre>
          <q-expansion-item
            v-if="promptPreview.sections.length"
            dense
            expand-separator
            icon="analytics"
            label="Token 分解（估算）"
            caption="构建期已含于上文；运行时按每轮对话追加"
            class="prompt-dialog__breakdown"
          >
            <AppRegistryTable
              :shell="false"
              :data-shell="true"
              hide-bottom
              row-key="key"
              :rows="promptPreview.sections"
              :columns="promptSectionColumns"
              hide-pagination
              :pagination="{ rowsPerPage: 0 }"
            >
              <template #body-cell-source="props">
                <q-td :props="props">
                  <q-chip dense size="sm" :color="props.row.source === 'build' ? 'primary' : 'secondary'" text-color="white">
                    {{ props.row.source === 'build' ? '构建期' : '运行时' }}
                  </q-chip>
                </q-td>
              </template>
              <template #body-cell-est_tokens="props">
                <q-td :props="props" class="text-right">
                  {{ props.row.est_tokens > 0 ? props.row.est_tokens : '—' }}
                </q-td>
              </template>
            </AppRegistryTable>
          </q-expansion-item>
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
import { computed, reactive, ref, toValue } from "vue";
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
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import { useAgentEvolutionPanel } from "../features/agents/useAgentEvolutionPanel";
import { useAgentA2AEndpointTab } from "../features/agents/useAgentA2AEndpointTab";
import { useAgentSettingsPage } from "../features/agents/useAgentSettingsPage";
import { tokenEstimateFor } from "../components/agents/agentUi";

import { AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS } from "../components/agents/agentTableUi";

const promptSectionColumns = AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS;

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

const {
  metricsLoading: evolutionMetricsLoading,
  metrics: evolutionMetrics,
  suggestions: evolutionSuggestions,
  applyingId: evolutionApplyingId,
  rejectingId: evolutionRejectingId,
  pendingSuggestionsCount: evolutionPendingCount,
  onApply: applyEvolutionSuggestion,
  onReject: rejectEvolutionSuggestion
} = useAgentEvolutionPanel(() => toValue(agentId), () => toValue(evolutionRange));

const a2aEndpoint = reactive(useAgentA2AEndpointTab(() => toValue(agentId)));

const promptInstructionText = computed(() => {
  const p = toValue(promptPreview);
  const instruction = String(p.instruction ?? "").trim();
  if (instruction) return instruction;
  const summary = String(p.summary ?? "").trim();
  return summary || "（当前模式下无 System Prompt 内容）";
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
    .filter((row) => row.source === "runtime" && row.est_tokens > 0)
    .reduce((sum, row) => sum + row.est_tokens, 0);
});

const advancedChannelOptions: { label: string; value: string }[] = [];
const loadingAdvancedChannels = ref(false);
</script>
