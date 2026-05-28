<template>
  <div class="tool-editor-policy-list">
    <q-banner v-if="registryLocked" rounded class="settings-warning-banner">
      内置 / 只读工具：「需确认 / 流式 / 并发」由代码 registry 维护，保存后重启可能被同步覆盖。日常启停请用列表页开关。
    </q-banner>

    <tool-policy-toggle-card
      v-for="item in toggles"
      :key="item.id"
      :copy="item"
      :model-value="Boolean(form[item.id])"
      :locked="registryLocked && Boolean(item.registryLocked)"
      :disable="item.id === 'readonly' && registryLocked"
      @update:model-value="patch({ [item.id]: $event })"
    />

    <q-banner rounded dense class="settings-info-banner">
      Agent 级并行、流式、重试在
      <router-link :to="{ name: 'agents' }" class="text-primary">Agent 列表</router-link>
      进入对应 Agent → 能力 Tab 配置；此处为 Tool 目录级标记。
    </q-banner>
  </div>
</template>

<script setup lang="ts">
import { TOOL_POLICY_TOGGLES } from "../../../features/tools/toolEditorCopy";
import { patchToolForm } from "../../../features/tools/toolFormPatch";
import type { ToolUpsertInput } from "../../../features/tools/types";
import ToolPolicyToggleCard from "./ToolPolicyToggleCard.vue";

const props = defineProps<{
  form: ToolUpsertInput;
  registryLocked: boolean;
}>();

const toggles = TOOL_POLICY_TOGGLES;

function patch(p: Partial<ToolUpsertInput>) {
  patchToolForm(props.form, p);
}
</script>
