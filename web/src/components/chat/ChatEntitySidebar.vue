<template>
  <transition name="chat-side">
    <aside v-show="open" class="chat-side chat-side--left column no-wrap">
      <div class="chat-side__header">
        <q-input
          :model-value="search"
          dense
          outlined
          clearable
          :dark="isDark"
          :placeholder="t('chat.searchPlaceholder')"
          class="chat-search"
          @update:model-value="$emit('update:search', String($event ?? ''))"
        >
          <template #prepend>
            <q-icon name="search" size="18px" />
          </template>
        </q-input>
      </div>

      <q-scroll-area class="col">
        <div class="chat-side__content">
          <q-item-label header class="chat-section-label text-cream-muted text-caption">
            {{ t("chat.groupAgents") }}
          </q-item-label>
          <div v-for="group in agentGroups" :key="group.key" class="chat-entity-group">
            <div class="chat-entity-group__label">
              <q-icon name="account_tree" size="14px" />
              <span>{{ group.label }}</span>
              <q-badge rounded color="primary" :label="group.items.length" />
            </div>
            <div class="column">
              <q-item
                v-for="agent in group.items"
                :key="agent.id"
                clickable
                :active="selectedKind === 'agent' && selectedAgentId === agent.id"
                :active-class="isDark ? 'bg-primary' : 'cream-menu-item--active'"
                class="chat-entity-item rounded-borders q-mb-sm"
                :class="{ 'chat-entity-item--active': selectedKind === 'agent' && selectedAgentId === agent.id }"
                @click="$emit('select-agent', agent)"
              >
                <q-item-section side class="chat-status-icon">
                  <q-icon
                    :name="isAgentWorking(agent) ? 'bolt' : 'task_alt'"
                    :color="isAgentWorking(agent) ? 'negative' : 'positive'"
                    size="xs"
                    dense
                  />
                </q-item-section>
                <q-item-section class="chat-entity-main">
                  <q-item-label class="chat-entity-name" lines="1">
                    {{ agent.display_name }}
                  </q-item-label>
                  <q-item-label caption class="chat-entity-meta">
                    <span class="chat-status-pill" :class="statusClass(agentStatus(agent))">
                      {{ statusLabel(agentStatus(agent)) }}
                    </span>
                  </q-item-label>
                </q-item-section>
                <q-item-section side class="chat-entity-actions">
                  <div class="chat-action-stack">
                    <q-btn
                      dense
                      round
                      flat
                      size="sm"
                      icon="settings"
                      class="chat-action-btn"
                      :aria-label="t('chat.settings')"
                      @click.stop="$emit('settings', 'agent', agent.id)"
                    />
                    <q-btn
                      dense
                      round
                      flat
                      size="sm"
                      color="negative"
                      icon="delete"
                      class="chat-action-btn chat-danger-btn"
                      :aria-label="t('chat.remove')"
                      @click.stop="$emit('delete', 'agent', agent.id)"
                    />
                  </div>
                </q-item-section>
              </q-item>
            </div>
          </div>
          <div v-if="agentGroups.length === 0" class="chat-side-hint text-caption text-cream-muted">
            没有匹配的 Agent
          </div>

          <q-item-label header class="text-cream-muted text-caption q-pt-md">
            {{ t("chat.groupTeams") }}
          </q-item-label>
          <div v-for="group in teamGroups" :key="group.key" class="chat-entity-group">
            <div class="chat-entity-group__label">
              <q-icon name="groups" size="14px" />
              <span>{{ group.label }}</span>
              <q-badge rounded color="primary" :label="group.items.length" />
            </div>
            <div class="column">
              <q-item
                v-for="team in group.items"
                :key="team.id"
                clickable
                :active="selectedKind === 'team' && selectedTeamId === team.id"
                :active-class="isDark ? 'bg-primary' : 'cream-menu-item--active'"
                class="chat-entity-item rounded-borders q-mb-sm"
                :class="{ 'chat-entity-item--active': selectedKind === 'team' && selectedTeamId === team.id }"
                @click="$emit('select-team', team)"
              >
                <q-item-section side class="chat-status-icon">
                  <q-icon
                    :name="team.isWorking ? 'bolt' : 'task_alt'"
                    :color="team.isWorking ? 'negative' : 'positive'"
                    size="xs"
                    dense
                  />
                </q-item-section>
                <q-item-section class="chat-entity-main">
                  <q-item-label class="chat-entity-name" lines="1">
                    {{ team.display_name }}
                  </q-item-label>
                  <q-item-label caption class="chat-entity-meta">
                    <span class="chat-status-pill" :class="statusClass(teamStatus(team))">
                      {{ statusLabel(teamStatus(team)) }}
                    </span>
                  </q-item-label>
                </q-item-section>
                <q-item-section side class="chat-entity-actions">
                  <div class="chat-action-stack">
                    <q-btn
                      dense
                      round
                      flat
                      size="sm"
                      icon="settings"
                      class="chat-action-btn"
                      :aria-label="t('chat.settings')"
                      @click.stop="$emit('settings', 'team', team.id)"
                    />
                    <q-btn
                      dense
                      round
                      flat
                      size="sm"
                      color="negative"
                      icon="delete"
                      class="chat-action-btn chat-danger-btn"
                      :aria-label="t('chat.remove')"
                      @click.stop="$emit('delete', 'team', team.id)"
                    />
                  </div>
                </q-item-section>
              </q-item>
            </div>
          </div>
          <div v-if="teamGroups.length === 0" class="chat-side-hint text-caption text-cream-muted">
            没有匹配的 Team
          </div>
        </div>
      </q-scroll-area>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { Agent, ChatEntityKind, DeleteKind, TeamRow } from "./types";
import type { PlatformResourceTreeNode } from "../../features/platform/api";

type EntityGroup<T> = {
  key: string;
  label: string;
  items: T[];
};

const props = defineProps<{
  open: boolean;
  search: string;
  agents: Agent[];
  teams: TeamRow[];
  categoryTree: PlatformResourceTreeNode[];
  selectedKind: ChatEntityKind;
  selectedAgentId?: string | null;
  selectedTeamId?: string | null;
  isDark: boolean;
}>();

defineEmits<{
  "update:search": [value: string];
  "update:agents": [value: Agent[]];
  "update:teams": [value: TeamRow[]];
  "agent-reorder-end": [];
  "team-reorder-end": [];
  "select-agent": [agent: Agent];
  "select-team": [team: TeamRow];
  settings: [kind: ChatEntityKind, id: string];
  delete: [kind: DeleteKind, id: string];
}>();

const { t } = useI18n();

const agentCategoryMap = computed(() => {
  const result = new Map<string, string>();
  for (const industry of props.categoryTree) {
    for (const department of industry.children ?? []) {
      for (const position of department.children ?? []) {
        result.set(position.id, `${industry.name} / ${department.name}`);
      }
    }
  }
  return result;
});

const agentGroups = computed(() => groupEntities(props.agents.filter(agentMatches), agentGroupLabel));
const teamGroups = computed(() => groupEntities(props.teams.filter(teamMatches), teamGroupLabel));

function normalizedSearch() {
  return props.search.trim().toLowerCase();
}

function agentMatches(agent: Agent) {
  const s = normalizedSearch();
  if (!s) return true;
  return agent.display_name.toLowerCase().includes(s) || agent.agent_key.toLowerCase().includes(s);
}

function teamMatches(team: TeamRow) {
  const s = normalizedSearch();
  if (!s) return true;
  return team.display_name.toLowerCase().includes(s);
}

function isAgentWorking(agent: Agent) {
  return /work|run|busy|ing/i.test(agent.status || "");
}

function agentStatus(agent: Agent) {
  if (isAgentWorking(agent)) return "working";
  if (/inactive|disabled|stop|pause/i.test(agent.status || "")) return "inactive";
  return "idle";
}

function teamStatus(team: TeamRow) {
  if (team.isWorking || /work|run|busy|ing/i.test(team.status || "")) return "working";
  if (/inactive|disabled|stop|pause/i.test(team.status || "")) return "inactive";
  return "idle";
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    working: "工作中",
    idle: "空闲",
    inactive: "已停用"
  };
  return labels[status] ?? status;
}

function statusClass(status: string) {
  return `is-${status}`;
}

function groupEntities<T extends { id: string }>(items: T[], labelFor: (item: T) => string): Array<EntityGroup<T>> {
  const groups = new Map<string, EntityGroup<T>>();
  for (const item of items) {
    const label = labelFor(item);
    const key = label.toLowerCase();
    if (!groups.has(key)) {
      groups.set(key, { key, label, items: [] });
    }
    groups.get(key)!.items.push(item);
  }
  return Array.from(groups.values());
}

function agentGroupLabel(agent: Agent) {
  if (!agent.category_position_id) return "未分类 Agent";
  return agentCategoryMap.value.get(agent.category_position_id) ?? "未分类 Agent";
}

function teamGroupLabel(team: TeamRow) {
  if (team.isDefault) return "默认 Team";
  const parsed = parseTeamDefinition(team.definition_json);
  return parsed.category || parsed.group || "自建 Team";
}

function parseTeamDefinition(raw?: string) {
  try {
    return JSON.parse(raw || "{}") as { category?: string; group?: string };
  } catch {
    return {};
  }
}
</script>

<style scoped>
.chat-side--left {
  width: var(--chat-side-left-width, 280px);
  min-width: min(var(--chat-side-left-width, 280px), 100%);
  flex: 0 0 var(--chat-side-left-width, 280px);
}

.chat-entity-group {
  margin-bottom: 10px;
}

.chat-entity-group__label {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  padding: 0 8px;
  color: var(--color-text-secondary);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.02em;
}

.chat-entity-group__label span {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-entity-item {
  align-items: center;
  min-height: 56px;
  padding: 8px 6px;
  color: var(--color-text-primary);
}

.chat-entity-name {
  display: block;
  max-width: 100%;
  overflow: hidden;
  color: inherit;
  font-size: 13px;
  font-weight: 700;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.body--dark) .chat-entity-item {
  color: var(--color-text-primary);
}

:global(.body--dark) .chat-entity-name {
  color: var(--color-text-primary);
  text-shadow: 0 1px 1px rgb(0 0 0 / 35%);
}

.chat-entity-item--active,
:global(.body--dark) .chat-entity-item--active {
  color: var(--color-on-accent) !important;
}

.chat-status-icon {
  min-width: 22px;
  padding-right: 4px;
}

.chat-entity-main {
  min-width: 0;
  flex: 1 1 auto;
  padding-right: 4px;
}

.chat-entity-actions {
  flex: 0 0 auto;
  min-width: 54px;
  padding-left: 2px;
}

.chat-action-stack {
  display: flex;
  align-items: center;
  gap: 4px;
}

.chat-action-btn {
  width: 24px;
  height: 24px;
  min-height: 24px;
  border-radius: 10px;
  background: rgb(255 255 255 / 72%);
}

:global(.body--dark) .chat-action-btn {
  color: rgb(248 250 252 / 92%);
  background: rgb(15 23 42 / 34%);
}

.chat-entity-item--active .chat-action-btn {
  color: var(--color-on-accent);
  background: rgb(255 255 255 / 18%);
}

.chat-entity-meta {
  margin-top: 3px;
}

.chat-status-pill {
  display: inline-flex;
  align-items: center;
  min-height: 18px;
  padding: 0 7px;
  border: 1px solid rgb(102 112 133 / 16%);
  border-radius: 999px;
  background: rgb(248 250 252 / 88%);
  color: var(--color-text-tertiary);
  font-size: 10px;
  font-weight: 800;
  line-height: 1;
}

.chat-status-pill.is-working {
  border-color: rgb(239 68 68 / 22%);
  background: rgb(254 242 242 / 92%);
  color: var(--color-danger-text);
}

.chat-status-pill.is-idle {
  border-color: rgb(34 197 94 / 22%);
  background: rgb(240 253 244 / 92%);
  color: var(--color-accent-green);
}

.chat-status-pill.is-inactive {
  border-color: rgb(102 112 133 / 20%);
  background: rgb(242 244 247 / 92%);
  color: var(--color-text-tertiary);
}

:global(.body--dark) .chat-entity-group__label,
:global(.body--dark) .chat-section-label,
:global(.body--dark) .chat-side-hint {
  color: var(--color-text-secondary) !important;
}

:global(.body--dark) .chat-status-pill {
  border-color: rgb(203 213 225 / 22%);
  background: rgb(15 23 42 / 46%);
  color: rgb(248 250 252 / 86%);
}

.chat-entity-item--active .chat-status-pill {
  border-color: rgb(255 255 255 / 35%);
  background: rgb(255 255 255 / 18%);
  color: var(--color-on-accent);
}
</style>
