<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['team-card', { 'is-dark': isDark }]">
    <q-card-section class="team-card__head">
      <div>
        <div class="row items-center q-gutter-sm">
          <div class="team-card__name text-h6 ellipsis">{{ team.display_name }}</div>
          <q-chip dense square color="primary" text-color="white">{{ definition.mode }}</q-chip>
          <q-chip v-if="team.is_default" dense square color="amber" text-color="black">默认</q-chip>
        </div>
        <button class="team-key" @click="$emit('copyKey', team.team_key)">{{ team.team_key }}</button>
      </div>
      <q-badge rounded :color="team.status === 'active' ? 'positive' : 'grey'">{{ team.status }}</q-badge>
    </q-card-section>

    <q-card-section class="q-pt-none">
      <p class="team-description">{{ definition.description || "暂无说明" }}</p>
      <div class="topology-strip">
        <div v-for="node in topologyNodesFromDefinition(definition)" :key="node.label" class="topology-node">
          <q-icon :name="node.icon" />
          <span>{{ node.label }}</span>
        </div>
      </div>
      <div class="member-list q-mt-md">
        <div v-for="member in definition.members" :key="`${team.id}-${member.agent_id}-${member.role}`" class="member-row">
          <q-avatar size="28px" color="primary" text-color="white" :icon="memberIcon(member.role)" />
          <div class="col min-width-0">
            <div class="member-name text-weight-medium ellipsis">{{ member.name || agentName(agents, member.agent_id) }}</div>
            <div class="member-meta text-caption">{{ member.role }} · {{ agentName(agents, member.agent_id) }}</div>
          </div>
          <q-badge rounded :color="member.enabled ? 'positive' : 'grey'">{{ member.enabled ? "启用" : "停用" }}</q-badge>
        </div>
        <div v-if="definition.members.length === 0" class="team-empty text-caption">尚未配置成员 Agent。</div>
      </div>
    </q-card-section>

    <q-separator />
    <q-card-actions align="between" class="team-card__actions">
      <span class="team-card__foot-meta text-caption">成员 {{ definition.members.length }} · {{ formatDate(team.updated_at) }}</span>
      <div class="q-gutter-xs">
        <q-btn flat dense round color="primary" icon="play_arrow" :to="`/chat?team=${team.id}`">
          <q-tooltip>进入 Chat 测试</q-tooltip>
        </q-btn>
        <q-btn flat dense round color="primary" icon="timeline" @click="$emit('openRuns', team)">
          <q-tooltip>查看运行轨迹</q-tooltip>
        </q-btn>
        <q-btn flat dense round color="primary" icon="content_copy" @click="$emit('duplicate', team)" />
        <q-btn flat dense round color="primary" icon="edit" @click="$emit('edit', team)" />
        <q-btn flat dense round color="negative" icon="delete" :disable="team.is_default" @click="$emit('remove', team)" />
      </div>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Agent } from "../../features/agents/api";
import type { Team } from "../../features/teams/api";
import { agentName, formatDate, memberIcon, parseDefinition, topologyNodesFromDefinition } from "./teamUtils";

const props = defineProps<{
  team: Team;
  agents: Agent[];
  isDark: boolean;
}>();

defineEmits<{
  copyKey: [value: string];
  openRuns: [team: Team];
  duplicate: [team: Team];
  edit: [team: Team];
  remove: [team: Team];
}>();

const definition = computed(() => parseDefinition(props.team));
</script>

<style scoped>
.team-card {
  overflow: hidden;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background: rgb(255 255 255 / 86%);
  box-shadow: 0 18px 48px rgb(16 24 40 / 6%);
  backdrop-filter: blur(16px);
}

.team-card__head,
.team-card__actions,
.member-row,
.topology-strip {
  display: flex;
  align-items: center;
}

.team-card__head,
.team-card__actions {
  justify-content: space-between;
  gap: 14px;
}

.team-key {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
}

.team-card__name {
  color: var(--color-text-dark);
  font-weight: 800;
}

.team-description {
  min-height: 42px;
  color: var(--color-text-tertiary);
  line-height: 1.6;
}

.member-name {
  color: var(--color-text-dark);
}

.member-meta,
.team-empty,
.team-card__foot-meta {
  color: var(--color-text-tertiary);
}

.topology-strip {
  gap: 8px;
  flex-wrap: wrap;
}

.topology-node {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  border: 1px solid rgb(25 118 210 / 12%);
  border-radius: 999px;
  background: var(--color-info-soft);
  color: var(--color-link);
  font-size: 12px;
  font-weight: 700;
}

.member-list {
  display: grid;
  gap: 8px;
}

.member-row {
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 16px;
  background: var(--color-page-tint);
}

.min-width-0 {
  min-width: 0;
}

.team-card.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 90%);
  box-shadow: 0 14px 38px rgb(0 0 0 / 32%);
}

.team-card.is-dark .team-key,
.team-card.is-dark .team-description,
.team-card.is-dark .member-meta,
.team-card.is-dark .team-empty,
.team-card.is-dark .team-card__foot-meta {
  color: var(--color-text-tertiary);
}

.team-card.is-dark .team-card__name,
.team-card.is-dark .member-name {
  color: var(--color-surface-soft);
  text-shadow: 0 1px 1px rgb(0 0 0 / 35%);
}

.team-card.is-dark .topology-node {
  border-color: rgb(96 165 250 / 22%);
  background: rgb(30 64 175 / 24%);
  color: var(--color-link);
}

.team-card.is-dark .member-row {
  border-color: rgb(148 163 184 / 14%);
  background: rgb(30 41 59 / 76%);
}
</style>
