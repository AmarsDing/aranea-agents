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
          <ChatSectionHeader
            icon="smart_toy"
            :label="t('chat.groupAgents')"
            :count="filteredAgents.length"
            :collapsed="collapse.sectionCollapsed.agents"
            @update:collapsed="collapse.toggleSection('agents')"
          />
          <template v-if="!collapse.sectionCollapsed.agents">
            <ChatEntityGroup
              v-for="group in agentGroups"
              :key="group.key"
              :items="group.items"
              :label="group.label"
              icon="account_tree"
              :collapsed="collapse.isGroupCollapsed(group.key)"
              :active-id="selectedKind === 'agent' ? selectedAgentId : null"
              :pinned-id="pinnedAgentId"
              settings-aria-label="设置"
              delete-aria-label="删除"
              @update:collapsed="collapse.toggleGroup(group.key)"
              @select="$emit('select-agent', $event as Agent)"
              @settings="$emit('settings', 'agent', $event)"
              @delete="$emit('delete', 'agent', $event)"
              @reorder="onAgentGroupReorder(group.key, $event)"
            />
            <div v-if="agentGroups.length === 0" class="chat-side-hint text-caption text-cream-muted">
              没有匹配的 Agent
            </div>
          </template>

          <ChatSectionHeader
            icon="groups"
            :label="t('chat.groupTeams')"
            :count="filteredTeams.length"
            :collapsed="collapse.sectionCollapsed.teams"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('teams')"
          />
          <template v-if="!collapse.sectionCollapsed.teams">
            <ChatEntityGroup
              v-for="group in teamGroups"
              :key="group.key"
              :items="group.items"
              :label="group.label"
              icon="groups"
              :collapsed="collapse.isGroupCollapsed(group.key)"
              :active-id="selectedKind === 'team' ? selectedTeamId : null"
              settings-aria-label="设置"
              delete-aria-label="删除"
              @update:collapsed="collapse.toggleGroup(group.key)"
              @select="$emit('select-team', $event as TeamRow)"
              @settings="$emit('settings', 'team', $event)"
              @delete="$emit('delete', 'team', $event)"
              @reorder="onTeamGroupReorder(group.key, $event)"
            />
            <div v-if="teamGroups.length === 0" class="chat-side-hint text-caption text-cream-muted">
              没有匹配的 Team
            </div>
          </template>
        </div>
      </q-scroll-area>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import ChatSectionHeader from "./ChatSectionHeader.vue";
import ChatEntityGroup from "./ChatEntityGroup.vue";
import type { ChatEntityKind, DeleteKind, TeamRow } from "./types";
import type { Agent } from "../../features/agents/types";
import type { PlatformResourceTreeNode } from "../../features/platform/types";
import { useChatEntityCollapse } from "../../features/chat/composables/useChatEntityCollapse";
import { loadGroupOrder } from "../../features/chat/composables/chatWorkspaceUtils";

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

const emit = defineEmits<{
  "update:search": [value: string];
  "update:agents": [value: Agent[]];
  "update:teams": [value: TeamRow[]];
  "agent-reorder-end": [];
  "team-reorder-end": [];
  "select-agent": [agent: Agent];
  "select-team": [team: TeamRow];
  settings: [kind: ChatEntityKind, id: string];
  delete: [kind: DeleteKind, id: string];
  "group-reorder": [groupKey: string, ids: string[]];
}>();

const { t } = useI18n();
const collapse = useChatEntityCollapse();

const pinnedAgentId = computed(() => {
  const pinned = props.agents.find((a) => a.is_default);
  return pinned?.id ?? null;
});

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

const filteredAgents = computed(() => props.agents.filter(agentMatches));
const filteredTeams = computed(() => props.teams.filter(teamMatches));

const agentGroups = computed(() => {
  const groups = groupEntities(filteredAgents.value, agentGroupLabel);
  return groups.map((g) => ({
    ...g,
    items: loadGroupOrder(g.items as Agent[], g.key, pinnedAgentId.value) as Agent[],
  }));
});

const teamGroups = computed(() => groupEntities(filteredTeams.value, teamGroupLabel));

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

function onAgentGroupReorder(groupKey: string, ids: string[]) {
  emit("group-reorder", groupKey, ids);
  emit("agent-reorder-end");
}

function onTeamGroupReorder(_groupKey: string, _ids: string[]) {
  emit("team-reorder-end");
}

watch(normalizedSearch, (s) => {
  if (s) {
    collapse.expandAllGroups();
  }
});
</script>

<style scoped>
.chat-side--left {
  width: var(--chat-side-left-width, 280px);
  min-width: min(var(--chat-side-left-width, 280px), 100%);
  flex: 0 0 var(--chat-side-left-width, 280px);
  overflow: hidden;
}

:global(.body--dark) .chat-side-hint {
  color: var(--chat-idle-meta);
}
</style>
