<template>
  <div v-if="agentId" class="agent-tools-section q-gutter-md">
    <div class="row items-center justify-between">
      <div class="text-body2 text-grey-7">
        按工具粒度覆盖启用、模式、需确认与配置 JSON（与 Tools 管理页一致；生效结果受上方「工具策略」总开关影响）。
      </div>
      <q-btn flat color="primary" icon="open_in_new" label="全局 Tools 管理" :to="{ name: 'tools' }" />
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
      @refresh="reload()"
      @edit="openEditor($event)"
      @remove="removeOverride($event)"
      @update:editor-open="editorOpen = $event"
      @update:form="form = $event"
      @save="saveOverride()"
    />
  </div>
</template>

<script setup lang="ts">
// Container: Agent 设置「能力」Tab 内工具覆盖区块；编排 useAgentToolOverrides，子 Panel 仅展示。
import { toRef } from "vue";
import AgentToolOverridesPanel from "./AgentToolOverridesPanel.vue";
import { useAgentToolOverrides } from "../../features/agents/useAgentToolOverrides";

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
  form,
  modeOptions,
  modeLabel,
  reload,
  openEditor,
  saveOverride,
  removeOverride
} = useAgentToolOverrides(toRef(props, "agentId"));
</script>
