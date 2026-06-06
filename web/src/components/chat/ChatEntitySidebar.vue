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
          <!-- Spirit Entry -->
          <SpiritEntry :active="selectedKind === 'spirit'" @click="$emit('select-spirit')" />

          <!-- Agent Section -->
          <ChatSectionHeader
            icon="smart_toy"
            :label="t('chat.groupAgents', 'Agent')"
            :count="filteredAgents.length"
            :collapsed="collapse.sectionCollapsed.agents"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('agents')"
          />
          <template v-if="!collapse.sectionCollapsed.agents">
            <ChatEntityGroup
              v-for="group in agentGroups"
              :key="group.key"
              :items="group.items"
              :label="group.label"
              :icon="group.icon"
              :collapsed="collapse.groupCollapsed[group.key] ?? false"
              :active-id="selectedAgentId"
              :pinned-id="group.pinnedId"
              @update:collapsed="collapse.toggleGroup(group.key)"
              @select="onSelectAgent"
              @settings="$emit('agent-settings', $event)"
              @delete="$emit('agent-delete', $event)"
              @reorder="(ids) => $emit('agent-reorder', { groupKey: group.key, ids })"
            />
            <div v-if="filteredAgents.length === 0" class="chat-side-hint text-caption text-cream-muted">
              暂无 Agent
            </div>
          </template>

          <!-- Active Teams Section -->
          <ChatSectionHeader
            icon="groups"
            :label="t('chat.groupActiveTeams', '进行中')"
            :count="activeTeamList.length"
            :collapsed="collapse.sectionCollapsed.activeTeams"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('activeTeams')"
          />
          <template v-if="!collapse.sectionCollapsed.activeTeams">
            <TeamTaskCard
              v-for="team in activeTeamList"
              :key="team.id"
              :team="team"
              :expanded="expandedTeamIds.has(team.id)"
              :active="selectedTeamId === team.id"
              @click="$emit('select-spirit-team', team.id)"
              @toggle-expand="$emit('toggle-team-expand', team.id)"
            />
            <div v-if="activeTeamList.length === 0" class="chat-side-hint text-caption text-cream-muted">
              暂无进行中的团队
            </div>
          </template>

          <!-- Completed Teams Section -->
          <ChatSectionHeader
            icon="check_circle"
            :label="t('chat.groupCompletedTeams', '已完成')"
            :count="completedTeamList.length"
            :collapsed="collapse.sectionCollapsed.completedTeams"
            class="q-pt-md"
            @update:collapsed="collapse.toggleSection('completedTeams')"
          />
          <template v-if="!collapse.sectionCollapsed.completedTeams">
            <TeamTaskCard
              v-for="team in completedTeamList"
              :key="team.id"
              :team="team"
              :expanded="expandedTeamIds.has(team.id)"
              :active="selectedTeamId === team.id"
              @click="$emit('select-spirit-team', team.id)"
              @toggle-expand="$emit('toggle-team-expand', team.id)"
            />
            <div v-if="completedTeamList.length === 0" class="chat-side-hint text-caption text-cream-muted">
              暂无已完成的团队
            </div>
          </template>
        </div>
      </q-scroll-area>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ChatSectionHeader from './ChatSectionHeader.vue';
import ChatEntityGroup from './ChatEntityGroup.vue';
import type { EntityItem } from './ChatEntityGroup.vue';
import SpiritEntry from '../spirit/SpiritEntry.vue';
import TeamTaskCard from '../spirit/TeamTaskCard.vue';
import type { SpiritTeam } from '../../features/spirit/types';
import type { Agent } from '../../features/agents/types';
import { useChatEntityCollapse } from '../../features/chat/composables/useChatEntityCollapse';
import { loadGroupOrder } from '../../features/chat/composables/chatWorkspaceUtils';

type AgentGroup = {
  key: string;
  label: string;
  icon: string;
  items: EntityItem[];
  pinnedId?: string;
};

const props = defineProps<{
  open: boolean;
  search: string;
  agents: Agent[];
  spiritTeams: SpiritTeam[];
  expandedTeamIds: Set<string>;
  selectedKind: string;
  selectedAgentId?: string | null;
  selectedTeamId?: string | null;
  defaultAgentId?: string | null;
  isDark: boolean;
}>();

const emit = defineEmits<{
  'update:search': [value: string];
  'select-spirit': [];
  'select-agent': [agent: Agent];
  'agent-settings': [id: string];
  'agent-delete': [id: string];
  'agent-reorder': [payload: { groupKey: string; ids: string[] }];
  'select-spirit-team': [teamId: string];
  'toggle-team-expand': [teamId: string];
}>();

const { t } = useI18n();
const collapse = useChatEntityCollapse();

// --- Search interaction with collapse ---
watch(
  () => props.search,
  (val, prev) => {
    if (val && !prev) {
      collapse.onSearchActive();
    } else if (!val && prev) {
      collapse.onSearchClear();
    }
  },
);

// --- Agent filtering ---
const filteredAgents = computed(() => {
  const q = props.search.trim().toLowerCase();
  if (!q) return props.agents;
  return props.agents.filter(
    (a) =>
      a.display_name.toLowerCase().includes(q) ||
      a.agent_key.toLowerCase().includes(q) ||
      a.agent_description?.toLowerCase().includes(q),
  );
});

// --- Agent grouping ---
const agentGroups = computed((): AgentGroup[] => {
  const agents = filteredAgents.value;
  if (agents.length === 0) return [];

  const defaultId = props.defaultAgentId;
  const defaultAgent = defaultId ? agents.find((a) => a.id === defaultId) : null;

  // Group 1: System/Default agents
  const systemAgents = agents.filter((a) => a.is_default);
  // Group 2: Custom agents (non-default)
  const customAgents = agents.filter((a) => !a.is_default);

  const groups: AgentGroup[] = [];

  if (systemAgents.length > 0) {
    const items = toEntityItems(loadGroupOrder(systemAgents, 'system', defaultId));
    groups.push({
      key: 'system',
      label: '系统 Agent',
      icon: 'verified',
      items,
      pinnedId: defaultId && systemAgents.some((a) => a.id === defaultId) ? defaultId : undefined,
    });
  }

  if (customAgents.length > 0) {
    const items = toEntityItems(loadGroupOrder(customAgents, 'custom'));
    groups.push({
      key: 'custom',
      label: '系统内置',
      icon: 'person',
      items,
    });
  }

  return groups;
});

function toEntityItems(agents: Agent[]): EntityItem[] {
  return agents.map((a) => ({
    id: a.id,
    display_name: a.display_name,
    status: a.status,
    is_default: a.is_default,
  }));
}

// --- Team lists ---
const activeTeamList = computed(() => props.spiritTeams.filter((t) => t.status !== 'completed'));
const completedTeamList = computed(() => props.spiritTeams.filter((t) => t.status === 'completed'));

// --- Agent selection ---
function onSelectAgent(item: EntityItem) {
  const agent = props.agents.find((a) => a.id === item.id);
  if (agent) {
    emit('select-agent', agent);
  }
}
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
