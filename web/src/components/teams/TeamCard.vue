<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['team-card', { 'is-dark': isDark }]">
    <div class="team-card__inner">
      <header class="team-card__head">
        <div class="team-card__head-main min-width-0">
          <div class="team-card__title-row">
            <h3 class="team-card__name ellipsis">{{ team.display_name }}</h3>
            <q-chip dense square size="sm" color="primary" text-color="white" class="team-card__mode-chip">
              {{ definition.mode }}
            </q-chip>
            <q-chip v-if="team.is_default" dense square size="sm" color="amber" text-color="black">默认</q-chip>
          </div>
          <button type="button" class="team-key" @click="$emit('copyKey', team.team_key)">{{ team.team_key }}</button>
        </div>
        <q-badge rounded class="team-card__status" :color="team.status === 'active' ? 'positive' : 'grey'">
          {{ team.status }}
        </q-badge>
      </header>

      <div class="team-card__meta">
        <p class="team-description">{{ definition.description || "暂无说明" }}</p>
        <div v-if="topologyNodes.length" class="topology-strip">
          <div v-for="node in topologyNodes" :key="node.label" class="topology-node">
            <q-icon :name="node.icon" size="14px" />
            <span>{{ node.label }}</span>
          </div>
        </div>
      </div>

      <div v-if="definition.members.length" class="member-list">
        <div
          v-for="member in definition.members"
          :key="`${team.id}-${member.agent_id}-${member.role}`"
          class="member-row"
        >
          <q-avatar size="26px" color="primary" text-color="white" :icon="memberIcon(member.role)" />
          <div class="member-primary ellipsis">
            <span class="member-role">{{ member.role }}</span>
            <span class="member-sep">·</span>
            <span class="member-label">{{ member.name || agentName(agents, member.agent_id) }}</span>
          </div>
          <q-badge dense rounded class="member-row__badge" :color="member.enabled ? 'positive' : 'grey'">
            {{ member.enabled ? "启用" : "停用" }}
          </q-badge>
        </div>
      </div>
      <div v-else class="team-empty">尚未配置成员 Agent。</div>

      <footer class="team-card__foot">
        <span class="team-card__foot-meta">成员 {{ definition.members.length }} · {{ formatDate(team.updated_at) }}</span>
        <div class="team-card__action-group">
          <q-btn flat dense round size="sm" color="primary" icon="account_tree" :to="`/teams/${team.id}/orchestrate`">
            <q-tooltip>编排 Graph</q-tooltip>
          </q-btn>
          <q-btn
            v-if="team.has_active_run"
            flat
            dense
            round
            size="sm"
            color="primary"
            icon="insights"
            @click="$emit('openObservatory', team)"
          >
            <q-tooltip>运行观测台</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="science" @click="$emit('runTest', team)">
            <q-tooltip>运行测试（API）</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="play_arrow" :to="`/chat?team=${team.id}`">
            <q-tooltip>进入 Chat 测试</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="timeline" @click="$emit('openRuns', team)">
            <q-tooltip>查看运行轨迹</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="content_copy" @click="$emit('duplicate', team)" />
          <q-btn flat dense round size="sm" color="primary" icon="edit" @click="$emit('edit', team)" />
          <q-btn flat dense round size="sm" color="negative" icon="delete" :disable="team.is_default" @click="$emit('remove', team)" />
        </div>
      </footer>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Agent } from "../../features/agents/types";
import type { Team } from "../../features/teams/types";
import { agentName, formatDate, memberIcon, parseDefinition, topologyNodesFromDefinition } from "./teamUtils";

const props = defineProps<{
  team: Team;
  agents: Agent[];
  isDark: boolean;
}>();

defineEmits<{
  copyKey: [value: string];
  openRuns: [team: Team];
  openObservatory: [team: Team];
  runTest: [team: Team];
  duplicate: [team: Team];
  edit: [team: Team];
  remove: [team: Team];
}>();

const definition = computed(() => parseDefinition(props.team));
const topologyNodes = computed(() => topologyNodesFromDefinition(definition.value));
</script>

<style scoped>
.team-card {
  overflow: hidden;
  border: 1px solid var(--glass-border);
  border-radius: 18px;
  background: var(--glass-surface);
  box-shadow: var(--shadow-entity-panel);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}

.team-card__inner {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px 12px;
}

.team-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.team-card__head-main {
  flex: 1;
  min-width: 0;
}

.team-card__title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 8px;
}

.team-card__name {
  margin: 0;
  color: var(--color-text-heading);
  font-size: 16px;
  font-weight: 800;
  line-height: 1.25;
}

.team-card__mode-chip {
  flex-shrink: 0;
}

.team-card__status {
  flex-shrink: 0;
  margin-top: 1px;
}

.team-key {
  display: inline-block;
  max-width: 100%;
  margin-top: 4px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  line-height: 1.3;
  text-align: left;
  transition: color 160ms ease;
}

.team-key:hover {
  color: var(--color-accent);
}

.team-card__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
}

.team-description {
  flex: 1 1 180px;
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 1.5;
}

.topology-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  flex-shrink: 0;
}

.topology-node {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border: 1px solid color-mix(in srgb, var(--color-accent) 18%, var(--glass-border));
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-accent) 6%, var(--glass-elevated));
  color: var(--color-link);
  font-size: 11px;
  font-weight: 700;
  line-height: 1.2;
  white-space: nowrap;
}

.member-list {
  display: grid;
  gap: 0;
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  overflow: hidden;
}

.member-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-elevated) 72%, transparent);
}

.member-row:last-child {
  border-bottom: none;
}

.member-primary {
  flex: 1;
  min-width: 0;
  color: var(--color-text-primary);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.3;
}

.member-role {
  color: var(--color-text-secondary);
  font-weight: 700;
  text-transform: lowercase;
}

.member-sep {
  margin: 0 3px;
  color: var(--color-text-tertiary);
}

.member-label {
  color: var(--color-text-heading);
}

.member-row__badge {
  flex-shrink: 0;
  font-size: 11px;
}

.team-empty {
  padding: 8px 10px;
  border: 1px dashed var(--glass-border);
  border-radius: 10px;
  color: var(--color-text-tertiary);
  font-size: 12px;
  line-height: 1.4;
  text-align: center;
}

.team-card__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--glass-border);
}

.team-card__foot-meta {
  color: var(--color-text-secondary);
  font-size: 11px;
  font-weight: 600;
  line-height: 1.3;
  white-space: nowrap;
}

.team-card__action-group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 2px;
}

.min-width-0 {
  min-width: 0;
}

.team-card.is-dark {
  border-color: var(--glass-border);
  background: var(--glass-surface);
  box-shadow: var(--shadow-entity-panel-dark);
}

.team-card.is-dark .topology-node {
  border-color: color-mix(in srgb, var(--color-accent) 24%, transparent);
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-elevated));
  color: var(--color-accent);
}

.team-card.is-dark .member-row {
  background: color-mix(in srgb, var(--glass-elevated) 55%, transparent);
}

@media (width <= 599px) {
  .team-card__foot {
    flex-direction: column;
    align-items: stretch;
  }

  .team-card__foot-meta {
    white-space: normal;
  }

  .team-card__action-group {
    justify-content: flex-start;
  }
}
</style>
