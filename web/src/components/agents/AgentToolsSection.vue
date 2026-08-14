<template>
  <div v-if="agentId" class="agent-tools-section">
    <div class="row items-center justify-between q-mb-sm">
      <p class="agent-tools-section__hint q-ma-none">
        {{ $t('toolsPage.agentTools.sectionSubtitle') }}
      </p>
      <q-btn flat rounded dense no-caps icon="open_in_new" :label="$t('toolsPage.agentTools.globalTools')" :to="{ name: 'tools' }" />
    </div>

    <agent-tool-overrides-panel
      :loading="loading"
      :saving="saving"
      :tools-enabled="toolsEnabled"
      :rows="rows"
      :editor-open="editorOpen"
      :editing-row="editingRow"
      :form="form"
      :mode-options="modeOptions"
      :mode-label="modeLabel"
      :effective-state-label="effectiveStateLabel"
      :confirm-remove-open="confirmRemoveOpen"
      :pending-remove-row="pendingRemoveRow"
      @refresh="reload()"
      @edit="openEditor($event)"
      @request-remove="requestRemoveOverride($event)"
      @confirm-remove="confirmRemoveOverride()"
      @cancel-remove="cancelRemoveOverride()"
      @update:editor-open="editorOpen = $event"
      @update:form="form = $event"
      @save="saveOverride()"
    />
  </div>
</template>

<script setup lang="ts">
// Container: Agent 设置「能力」Tab 内工具覆盖区块；编排 useAgentToolOverrides，子 Panel 仅展示。
import { toRef } from 'vue';
import AgentToolOverridesPanel from './AgentToolOverridesPanel.vue';
import { useAgentToolOverrides } from '../../features/agents/useAgentToolOverrides';

const props = defineProps<{
  agentId: string;
}>();

const {
  loading,
  saving,
  toolsEnabled,
  rows,
  editorOpen,
  editingRow,
  confirmRemoveOpen,
  pendingRemoveRow,
  form,
  modeOptions,
  modeLabel,
  effectiveStateLabel,
  reload,
  openEditor,
  saveOverride,
  requestRemoveOverride,
  confirmRemoveOverride,
  cancelRemoveOverride,
} = useAgentToolOverrides(toRef(props, 'agentId'));
</script>
