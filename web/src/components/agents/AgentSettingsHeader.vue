<template>
  <q-card-section class="agent-settings-header settings-header">
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
          <q-badge rounded :class="['settings-status', agent.status === 'active' ? 'is-active' : '']">{{ statusLabel(agent.status) }}</q-badge>
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
      <q-btn color="primary" rounded unelevated icon="save" label="保存设置" class="settings-save" :loading="saving" :disable="!agent.id" @click="$emit('save')" />
    </div>
  </q-card-section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Agent } from "../../features/agents/types";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { promptModeLabel, statusLabel } from "./agentUi";

const props = defineProps<{
  agent: Agent;
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
