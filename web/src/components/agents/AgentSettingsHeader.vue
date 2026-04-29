<template>
  <q-card-section class="settings-header">
    <div class="row items-center q-gutter-md no-wrap">
      <q-btn flat round icon="arrow_back" class="header-icon-btn" @click="$emit('back')" />
      <agent-avatar-q
        size="64px"
        avatar-class="settings-avatar cursor-pointer"
        :icon="agent.icon"
        :alt="agent.display_name || 'Agent 设置'"
        @click="$emit('change-avatar')"
      >
        <q-tooltip>头像选择器将在 Avatar 流程中接入</q-tooltip>
      </agent-avatar-q>
      <div class="min-width-0">
        <div class="row items-center q-gutter-sm">
          <div class="text-h5 text-weight-bold ellipsis">{{ agent.display_name || "Agent 设置" }}</div>
          <q-badge rounded :class="['settings-status', agent.status === 'active' ? 'is-active' : '']">{{ agent.status }}</q-badge>
          <q-chip dense square class="settings-chip">{{ promptModeLabel(agent.system_prompt_mode) }}</q-chip>
          <q-chip v-if="selfEvolve" dense square class="settings-chip is-evolving" icon="auto_awesome">进化中</q-chip>
        </div>
        <div class="text-caption text-grey-7">{{ agent.agent_key }} · {{ agent.provider }} / {{ agent.model }}</div>
      </div>
    </div>

    <div class="row q-gutter-sm">
      <q-btn outline rounded color="primary" icon="visibility" label="系统提示词" class="settings-action" @click="$emit('open-prompt')" />
      <q-btn flat round color="amber-8" :icon="favorite ? 'star' : 'star_border'" class="header-icon-btn" @click="$emit('toggle-favorite')" />
      <q-btn color="primary" rounded unelevated icon="save" label="保存设置" class="settings-save" :loading="saving" @click="$emit('save')" />
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import type { Agent } from "../../features/agents/api";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { promptModeLabel } from "./agentUi";

defineProps<{
  agent: Agent;
  selfEvolve: boolean;
  favorite: boolean;
  saving: boolean;
}>();

defineEmits<{
  back: [];
  "change-avatar": [];
  "open-prompt": [];
  "toggle-favorite": [];
  save: [];
}>();
</script>

<style scoped>
.settings-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 22px 24px;
  background:
    radial-gradient(circle at top left, rgba(25, 118, 210, 0.08), transparent 28%),
    linear-gradient(180deg, #ffffff, #fbfcff);
}

.settings-avatar {
  box-shadow: 0 14px 34px rgba(25, 118, 210, 0.2);
}

.header-icon-btn {
  background: rgba(248, 250, 252, 0.92);
}

.settings-status {
  padding: 4px 8px;
  background: #eef2f6;
  color: #475467;
  font-weight: 700;
  text-transform: capitalize;
}

.settings-status.is-active {
  background: #e7f8ef;
  color: #027a48;
}

.settings-chip {
  border: 1px solid rgba(245, 158, 11, 0.18);
  background: #fff7ed;
  color: #b45309;
  font-weight: 700;
}

.settings-chip.is-evolving {
  background: #fff4e5;
}

.settings-action,
.settings-save {
  min-height: 40px;
  padding: 0 16px;
  font-weight: 700;
}

.settings-save {
  box-shadow: 0 12px 26px rgba(25, 118, 210, 0.2);
}

.min-width-0 {
  min-width: 0;
}

body.body--dark .settings-header {
  background:
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.16), transparent 30%),
    linear-gradient(180deg, #1f2937, #111827);
}

body.body--dark .header-icon-btn {
  background: rgba(15, 23, 42, 0.74);
}

body.body--dark .settings-status {
  background: rgba(148, 163, 184, 0.16);
  color: #cbd5e1;
}

body.body--dark .settings-status.is-active {
  background: rgba(16, 185, 129, 0.18);
  color: #86efac;
}

body.body--dark .settings-chip {
  border-color: rgba(251, 191, 36, 0.24);
  background: rgba(120, 53, 15, 0.32);
  color: #fcd34d;
}

body.body--dark .settings-chip.is-evolving {
  background: rgba(146, 64, 14, 0.32);
}

body.body--dark .settings-header :deep(.text-grey-7) {
  color: #94a3b8 !important;
}
</style>
