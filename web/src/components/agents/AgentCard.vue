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
      </div>
    </q-card-section>

    <q-space />
    <q-separator />
    <q-card-actions align="between" class="agent-card__actions">
      <span class="agent-card__context">{{ contextLabel }}</span>
      <div class="q-gutter-xs">
        <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${agent.id}/settings`" />
        <q-btn flat dense rounded color="negative" icon="delete" label="删除" @click="$emit('delete', agent)" />
      </div>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../../features/agents/api";
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
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
</script>

<style scoped>
.agent-card {
  display: flex;
  flex-direction: column;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 24px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(255, 255, 255, 0.9)),
    radial-gradient(circle at top right, rgba(25, 118, 210, 0.08), transparent 30%);
  overflow: hidden;
  box-shadow: 0 14px 36px rgba(16, 24, 40, 0.045);
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    border-color 180ms ease;
}

.agent-card:hover {
  transform: translateY(-2px);
  border-color: rgba(25, 118, 210, 0.32);
  box-shadow: 0 22px 56px rgba(16, 24, 40, 0.1);
}

.agent-card__header {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: nowrap;
}

.agent-card__avatar {
  box-shadow: 0 12px 28px rgba(25, 118, 210, 0.2);
}

.agent-card__name {
  color: #101828;
}

.agent-card__status {
  padding: 4px 8px;
  background: #eef2f6;
  color: #475467;
  font-weight: 700;
  text-transform: capitalize;
}

.agent-card__status.is-active {
  background: #e7f8ef;
  color: #027a48;
}

.agent-card__model {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: #667085;
  font-size: 12px;
  font-weight: 600;
}

.agent-description {
  min-height: 52px;
  margin: 12px 0;
  color: #475467;
  font-size: 13px;
  line-height: 1.6;
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.agent-card__chip {
  border: 1px solid rgba(15, 23, 42, 0.08);
  background: #f8fafc;
  color: #475467;
  font-weight: 600;
}

.agent-card__chip.is-evolving {
  border-color: rgba(245, 158, 11, 0.22);
  background: #fff7ed;
  color: #b45309;
}

.agent-handle {
  padding: 0;
  border: 0;
  background: transparent;
  color: #667085;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
}

.agent-card__actions {
  padding: 12px 16px;
  background: rgba(248, 250, 252, 0.72);
}

.agent-card__context {
  color: #475467;
  font-size: 12px;
  font-weight: 700;
}

.min-width-0 {
  min-width: 0;
}

.agent-card.agent-card--dark {
  border-color: rgba(148, 163, 184, 0.16);
  background:
    linear-gradient(180deg, rgba(17, 24, 39, 0.96), rgba(15, 23, 42, 0.9)),
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 32%);
  box-shadow: 0 16px 42px rgba(0, 0, 0, 0.3);
}

.agent-card.agent-card--dark:hover {
  border-color: rgba(96, 165, 250, 0.38);
  box-shadow: 0 22px 56px rgba(0, 0, 0, 0.42);
}

.agent-card.agent-card--dark .agent-card__status {
  background: rgba(51, 65, 85, 0.78);
  color: #cbd5e1;
}

.agent-card.agent-card--dark .agent-card__status.is-active {
  background: rgba(22, 101, 52, 0.28);
  color: #86efac;
}

.agent-card.agent-card--dark .agent-card__name {
  color: #f8fafc;
  text-shadow: 0 1px 1px rgba(0, 0, 0, 0.35);
}

.agent-card.agent-card--dark .agent-card__model,
.agent-card.agent-card--dark .agent-description,
.agent-card.agent-card--dark .agent-handle,
.agent-card.agent-card--dark .agent-card__context {
  color: #94a3b8;
}

.agent-card.agent-card--dark .agent-card__chip {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(30, 41, 59, 0.78);
  color: #cbd5e1;
}

.agent-card.agent-card--dark .agent-card__chip.is-evolving {
  border-color: rgba(245, 158, 11, 0.28);
  background: rgba(120, 53, 15, 0.26);
  color: #fbbf24;
}

.agent-card.agent-card--dark .agent-card__actions {
  background: rgba(15, 23, 42, 0.74);
}
</style>
