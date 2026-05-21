<template>
  <q-card-section class="settings-header">
    <div class="row items-center q-gutter-md no-wrap">
      <q-btn flat round icon="arrow_back" class="header-icon-btn" @click="$emit('back')" />
      <div class="settings-header__avatar-wrap cursor-pointer" @click="$emit('change-avatar')">
        <agent-avatar-q
          :key="`${agent.id}:${agent.icon ?? ''}`"
          size="64px"
          avatar-class="settings-avatar"
          :icon="agent.icon"
          :alt="agent.display_name || 'Agent 设置'"
        />
        <q-tooltip>点击更换头像（内置或上传）</q-tooltip>
      </div>
      <div class="min-width-0">
        <div class="row items-center q-gutter-sm">
          <div class="text-h5 text-weight-bold ellipsis">{{ agent.display_name || "Agent 设置" }}</div>
          <q-badge rounded :class="['settings-status', agent.status === 'active' ? 'is-active' : '']">{{ agent.status }}</q-badge>
          <q-chip dense square class="settings-chip">{{ promptModeLabel(agent.system_prompt_mode) }}</q-chip>
          <q-chip v-if="showEvolving" dense square class="settings-chip is-evolving" icon="auto_awesome">进化中</q-chip>
        </div>
        <div v-if="metaCaption" class="text-caption text-grey-7">{{ metaCaption }}</div>
      </div>
    </div>

    <div class="row q-gutter-sm">
      <q-btn outline rounded color="primary" icon="visibility" label="系统提示词" class="settings-action" @click="$emit('open-prompt')" />
      <q-btn outline rounded color="grey-7" icon="settings" label="高级" class="settings-action" @click="$emit('open-advanced')" />
      <q-btn flat round color="amber-8" :icon="favorite ? 'star' : 'star_border'" class="header-icon-btn" @click="$emit('toggle-favorite')" />
      <q-btn color="primary" rounded unelevated icon="save" label="保存设置" class="settings-save" :loading="saving" @click="$emit('save')" />
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Agent } from "../../features/agents/api";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { promptModeLabel } from "./agentUi";

const props = defineProps<{
  agent: Agent;
  selfEvolve: boolean;
  showEvolving: boolean;
  favorite: boolean;
  saving: boolean;
}>();

/** 过滤误写入的相对路径式 agent_key（如 `./`），避免副标题出现怪字符串 */
function metaCaptionForAgent(agent: Agent): string {
  const key = String(agent.agent_key ?? "").trim();
  const junkKey = !key || /^\.{1,2}(\/.*)?$/.test(key) || /[\\/]/.test(key);
  const provider = String(agent.provider ?? "").trim();
  const model = String(agent.model ?? "").trim();
  const pm = provider && model ? `${provider} / ${model}` : provider || model || "";
  if (!junkKey && pm) return `${key} · ${pm}`;
  if (!junkKey) return key;
  return pm;
}

const metaCaption = computed(() => metaCaptionForAgent(props.agent));

defineEmits<{
  back: [];
  "change-avatar": [];
  "open-prompt": [];
  "open-advanced": [];
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
  background: var(--glass-elevated);
  border-bottom: 1px solid var(--glass-border);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.settings-avatar {
  box-shadow: 0 14px 34px rgb(25 118 210 / 20%);
}

.header-icon-btn {
  background: rgb(248 250 252 / 92%);
}

.settings-status {
  padding: 4px 8px;
  background: var(--color-status-info-bg);
  color: var(--color-text-tertiary);
  font-weight: 700;
  text-transform: capitalize;
}

.settings-status.is-active {
  background: var(--color-status-success-bg);
  color: var(--color-accent-green);
}

.settings-chip {
  border: 1px solid rgb(245 158 11 / 18%);
  background: var(--color-status-warning-bg);
  color: var(--color-status-warning-text);
  font-weight: 700;
}

.settings-chip.is-evolving {
  background: var(--color-status-warning-bg-alt);
}

.settings-action,
.settings-save {
  min-height: 40px;
  padding: 0 16px;
  font-weight: 700;
}

.settings-save {
  box-shadow: 0 12px 26px rgb(25 118 210 / 20%);
}

.min-width-0 {
  min-width: 0;
}

.settings-header__avatar-wrap {
  flex-shrink: 0;
  line-height: 0;
}

body.body--dark .settings-header {
  background: var(--glass-elevated);
  border-bottom-color: var(--glass-border);
}

body.body--dark .header-icon-btn {
  background: rgb(15 23 42 / 74%);
}

body.body--dark .settings-status {
  background: rgb(148 163 184 / 16%);
  color: var(--color-text-slate-300);
}

body.body--dark .settings-status.is-active {
  background: rgb(16 185 129 / 18%);
  color: var(--color-accent-green);
}

body.body--dark .settings-chip {
  border-color: rgb(251 191 36 / 24%);
  background: rgb(120 53 15 / 32%);
  color: var(--color-accent-amber-light);
}

body.body--dark .settings-chip.is-evolving {
  background: rgb(146 64 14 / 32%);
}

body.body--dark .settings-header :deep(.text-grey-7) {
  color: var(--color-text-tertiary) !important;
}
</style>
