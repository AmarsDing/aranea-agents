<template>
  <q-card flat bordered :class="['agent-card full-height', { 'agent-card--dark': isDark }]">
    <q-card-section class="agent-card__header">
      <agent-avatar-q :icon="agent.icon" :alt="agent.display_name" size="56px" avatar-class="agent-card__avatar" />
      <div class="col min-width-0">
        <div class="row items-center no-wrap q-gutter-xs">
          <q-btn
            flat
            dense
            round
            size="sm"
            :aria-label="favorite ? '取消收藏' : '收藏 Agent'"
            :color="favorite ? 'amber-8' : 'grey-5'"
            :icon="favorite ? 'star' : 'star_border'"
            @click="$emit('toggle-favorite', agent.id)"
          />
          <div class="agent-card__name text-subtitle1 text-weight-bold ellipsis">{{ agent.display_name }}</div>
        </div>
        <button class="agent-handle" @click="$emit('copy-key', agent.agent_key)">{{ agent.agent_key }}</button>
      </div>
      <q-badge rounded :class="['agent-card__status', agent.status === 'active' ? 'is-active' : '']">{{ agent.status }}</q-badge>
    </q-card-section>

    <q-card-section class="q-pt-none">
      <div class="agent-card__model"><q-icon name="memory" size="14px" />{{ agent.provider }} / {{ agent.model }}</div>
      <p class="agent-description">{{ agent.agent_description || "暂无描述，可在设置中补充能力边界与使用场景。" }}</p>
      <div class="row q-gutter-xs">
        <q-chip dense square class="agent-card__chip">{{ categoryLabel }}</q-chip>
        <q-chip v-if="evolving" dense square class="agent-card__chip is-evolving" icon="auto_awesome">进化中</q-chip>
        <q-chip v-if="agent.agent_kind === 'a2a_proxy'" dense square class="agent-card__chip is-a2a-proxy" icon="sync_alt">A2A ↗</q-chip>
        <q-chip
          v-else-if="agent.a2a_endpoint_enabled"
          dense
          square
          class="agent-card__chip is-a2a-endpoint"
          icon="call_received"
        >
          A2A ↙
        </q-chip>
      </div>
    </q-card-section>

    <q-space />
    <q-separator />
    <q-card-actions align="between" class="agent-card__actions">
      <span class="agent-card__context">{{ contextLabel }}</span>
      <div class="q-gutter-xs">
        <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${agent.id}/settings`" />
        <q-btn flat dense rounded color="secondary" label="复制" @click="$emit('duplicate', agent)" />
        <q-btn flat dense rounded color="negative" icon="delete" label="删除" @click="$emit('delete', agent)" />
      </div>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../../features/agents/types";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";

const props = defineProps<{
  agent: Agent;
  favorite: boolean;
  categoryLabel: string;
  contextLabel: string;
  evolving: boolean;
}>();

defineEmits<{
  "toggle-favorite": [id: string];
  "copy-key": [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
</script>

<style scoped>
.agent-card {
  display: flex;
  flex-direction: column;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background:
    linear-gradient(180deg, rgb(255 255 255 / 98%), rgb(255 255 255 / 90%)),
    radial-gradient(circle at top right, rgb(25 118 210 / 8%), transparent 30%);
  overflow: hidden;
  box-shadow: 0 14px 36px rgb(16 24 40 / 4.5%);
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease;
}

.agent-card:hover {
  transform: translateY(-2px);
  border-color: rgb(25 118 210 / 32%);
  box-shadow: 0 22px 56px rgb(16 24 40 / 10%);
}

.agent-card__header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: nowrap;
}

.agent-card__avatar {
  box-shadow: 0 12px 28px rgb(25 118 210 / 20%);
}

.agent-card__name {
  color: var(--color-text-dark);
}

.agent-card__status {
  padding: 4px 8px;
  background: var(--color-status-info-bg);
  color: var(--color-text-tertiary);
  font-weight: 700;
  text-transform: capitalize;
}

.agent-card__status.is-active {
  background: var(--color-status-success-bg);
  color: var(--color-accent-green);
}

.agent-card__model {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-tertiary);
  font-size: 12px;
  font-weight: 600;
}

.agent-description {
  min-height: 52px;
  margin: 12px 0;
  color: var(--color-text-tertiary);
  font-size: 13px;
  line-height: 1.6;
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.agent-card__chip {
  border: 1px solid rgb(15 23 42 / 8%);
  background: var(--color-surface-soft);
  color: var(--color-text-tertiary);
  font-weight: 600;
}

.agent-card__chip.is-evolving {
  border-color: rgb(245 158 11 / 22%);
  background: var(--color-status-warning-bg);
  color: var(--color-status-warning-text);
}

.agent-card__chip.is-a2a-proxy,
.agent-card__chip.is-a2a-endpoint {
  border-color: rgb(25 118 210 / 22%);
  background: rgb(239 246 255 / 92%);
  color: rgb(29 78 216);
}

.agent-handle {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
}

.agent-card__actions {
  padding: 12px 16px;
  background: rgb(248 250 252 / 72%);
}

.agent-card__context {
  color: var(--color-text-tertiary);
  font-size: 12px;
  font-weight: 700;
}

.min-width-0 {
  min-width: 0;
}

.agent-card.agent-card--dark {
  border-color: rgb(148 163 184 / 16%);
  background:
    linear-gradient(180deg, rgb(17 24 39 / 96%), rgb(15 23 42 / 90%)),
    radial-gradient(circle at top right, rgb(59 130 246 / 14%), transparent 32%);
  box-shadow: 0 16px 42px rgb(0 0 0 / 30%);
}

.agent-card.agent-card--dark:hover {
  border-color: rgb(96 165 250 / 38%);
  box-shadow: 0 22px 56px rgb(0 0 0 / 42%);
}

.agent-card.agent-card--dark .agent-card__status {
  background: rgb(51 65 85 / 78%);
  color: var(--color-text-slate-300);
}

.agent-card.agent-card--dark .agent-card__status.is-active {
  background: rgb(22 101 52 / 28%);
  color: var(--color-accent-green);
}

.agent-card.agent-card--dark .agent-card__name {
  color: var(--color-surface-soft);
  text-shadow: 0 1px 1px rgb(0 0 0 / 35%);
}

.agent-card.agent-card--dark .agent-card__model,
.agent-card.agent-card--dark .agent-description,
.agent-card.agent-card--dark .agent-handle,
.agent-card.agent-card--dark .agent-card__context {
  color: var(--color-text-tertiary);
}

.agent-card.agent-card--dark .agent-card__chip {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(30 41 59 / 78%);
  color: var(--color-text-slate-300);
}

.agent-card.agent-card--dark .agent-card__chip.is-evolving {
  border-color: rgb(245 158 11 / 28%);
  background: rgb(120 53 15 / 26%);
  color: var(--color-accent-amber);
}

.agent-card.agent-card--dark .agent-card__chip.is-a2a-proxy,
.agent-card.agent-card--dark .agent-card__chip.is-a2a-endpoint {
  border-color: rgb(59 130 246 / 28%);
  background: rgb(30 58 138 / 26%);
  color: rgb(147 197 253);
}

.agent-card.agent-card--dark .agent-card__actions {
  background: rgb(15 23 42 / 74%);
}
</style>
