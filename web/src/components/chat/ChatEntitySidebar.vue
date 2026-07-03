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
          <SpiritEntry
            :active="selectedKind === 'spirit'"
            :icon="spiritAgentIcon"
            :show-settings="!!spiritAgentId"
            @click="$emit('select-spirit')"
            @settings="spiritAgentId && $emit('spirit-settings', spiritAgentId)"
          />

          <!-- B.9.1: Agent activity card list (below Spirit entry, no grouping/folding, ordered by creation time). -->
          <div class="agent-card-list">
            <!-- Orchestration phase progress (shown before teams arrive) -->
            <div v-if="showOrchestrationProgress && phaseDisplay" class="spirit-phase-hint q-px-sm q-py-xs">
              <q-icon :name="phaseDisplay.icon" size="16px" :style="{ color: phaseDisplay.color }" class="q-mr-xs" />
              <span class="text-caption">{{ phaseDisplay.label }}</span>
              <q-spinner-dots size="14px" :color="phaseDisplay.color" class="q-ml-xs" />
            </div>
            <AgentSidebarCard
              v-for="card in allAgentCards"
              :key="`${card.teamId}-${card.member.agentKey}`"
              :member="card.member"
              :team-name="card.teamName"
              :team-session-id="card.teamSessionId"
              :team-status="card.teamStatus"
              :team-id="card.teamId"
              :blocked-info="getBlockedInfoForAgent(card.member.agentKey)"
              @locate="onLocateAgent"
              @pause="$emit('pause-agent', $event)"
              @resume="$emit('resume-agent', $event)"
              @settings="onAgentSettings"
            />
            <div
              v-if="allAgentCards.length === 0 && !showOrchestrationProgress"
              class="chat-side-hint text-caption text-cream-muted"
            >
              {{ t('chat.sidebar.noActiveTeams') }}
            </div>
          </div>
        </div>
      </q-scroll-area>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import SpiritEntry from '../spirit/SpiritEntry.vue';
import AgentSidebarCard from './AgentSidebarCard.vue';
import type { SpiritTeam, SpiritMember, SpiritTeamStatus } from '../../features/spirit/types';
import type { Agent } from '../../features/agents/types';
import type { BlockedResult } from '../../features/chat/composables/useBlockedStatus';
import { EMPTY_BLOCKED } from '../../features/chat/composables/useBlockedStatus';
import { useChatEntityCollapse } from '../../features/chat/composables/useChatEntityCollapse';

const props = defineProps<{
  open: boolean;
  search: string;
  agents: Agent[];
  spiritTeams: SpiritTeam[];
  selectedKind: string;
  isDark: boolean;
  /** Current orchestration phase for sidebar progress display. */
  orchestrationPhase?: string;
  /** 当前活动树的阻塞检测结果，用于精确高亮左侧 Agent 卡片 */
  blockedStatus?: BlockedResult;
}>();

const emit = defineEmits<{
  'update:search': [value: string];
  'select-spirit': [];
  'spirit-settings': [id: string];
  'agent-settings': [id: string];
  'locate-agent': [payload: { agentKey: string; teamSessionId: string; teamId: string }];
  'pause-agent': [agentKey: string];
  'resume-agent': [agentKey: string];
  'cancel-agent': [agentKey: string];
}>();

const { t } = useI18n();
const collapse = useChatEntityCollapse();

// --- Spirit agent icon ---
const spiritAgentId = computed(() => {
  const spirit = props.agents.find((a) => a.agent_key === '__spirit__');
  return spirit?.id ?? null;
});
const spiritAgentIcon = computed(() => {
  const spirit = props.agents.find((a) => a.agent_key === '__spirit__');
  return spirit?.icon ?? '';
});

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

// --- Orchestration phase display ---
const PHASE_DISPLAY: Record<string, { icon: string; label: string; color: string }> = {
  planning: { icon: 'search', label: '正在规划任务…', color: 'var(--color-accent)' },
  allocating: { icon: 'people', label: '正在分配 Agent…', color: 'var(--color-warning)' },
  orchestrating: { icon: 'construction', label: '正在编排执行…', color: 'var(--color-accent)' },
};

const showOrchestrationProgress = computed(() => {
  const phase = props.orchestrationPhase;
  return phase && phase !== 'idle' && phase !== 'completed' && phase !== 'interrupted';
});

const phaseDisplay = computed(() => {
  const phase = props.orchestrationPhase;
  return phase ? PHASE_DISPLAY[phase] : null;
});

// --- Agent 活动卡片列表：从所有 spiritTeams 中提取成员，按创建顺序排列 ---
type AgentCardData = {
  teamId: string;
  teamName: string;
  teamSessionId: string;
  teamStatus: SpiritTeamStatus;
  member: SpiritMember;
};

const allAgentCards = computed<AgentCardData[]>(() => {
  const q = props.search.trim().toLowerCase();
  const cards: AgentCardData[] = [];
  // B.9.1: 按团队创建时间排序，确保 Agent 卡片按创建顺序展示
  const sortedTeams = [...props.spiritTeams].sort((a, b) => a.createdAt - b.createdAt);
  for (const team of sortedTeams) {
    // 搜索过滤
    if (q && !team.teamName.toLowerCase().includes(q) && !team.taskSummary.toLowerCase().includes(q)) {
      // 仍然搜索成员名
      const hasMatchingMember = team.members.some(
        (m) => m.displayName.toLowerCase().includes(q) || m.agentKey.toLowerCase().includes(q),
      );
      if (!hasMatchingMember) continue;
    }
    for (const member of team.members) {
      cards.push({
        teamId: team.id,
        teamName: team.teamName,
        teamSessionId: team.teamSessionId,
        teamStatus: team.status,
        member,
      });
    }
  }
  return cards;
});

function onLocateAgent(payload: { agentKey: string; teamSessionId: string; teamId: string }) {
  emit('locate-agent', payload);
}

/** 左侧 Agent 卡片设置按钮：按 agentKey 查找 Agent 配置，转发 agentId 给外层打开设置弹窗 */
function onAgentSettings(agentKey: string) {
  const agent = props.agents.find((a) => a.agent_key === agentKey);
  if (agent?.id) {
    emit('agent-settings', agent.id);
  }
}

/** 根据当前活动树的阻塞检测结果，判断指定 agentKey 是否处于阻塞态 */
function getBlockedInfoForAgent(agentKey: string): BlockedResult {
  const blocked = props.blockedStatus;
  if (!blocked?.blocked) return EMPTY_BLOCKED;
  return blocked.agentKey === agentKey ? blocked : EMPTY_BLOCKED;
}
</script>

<style scoped>
.chat-side--left {
  width: var(--chat-side-left-width, 280px);
  min-width: min(var(--chat-side-left-width, 280px), 100%);
  flex: 0 0 var(--chat-side-left-width, 280px);
  overflow: hidden;
}

/* Agent 卡片列表：垂直布局 + 上下左右间隔 */
.agent-card-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 6px 10px;
}

:global(.body--dark) .chat-side-hint {
  color: var(--chat-idle-meta);
}

.spirit-phase-hint {
  display: flex;
  align-items: center;
  opacity: 0.85;
}

.team-card-wrapper--pulse {
  animation: status-pulse var(--pulse-duration, 1.5s) ease-out;
  border-left: 2px solid var(--pulse-color);
  border-radius: 12px;
}

@keyframes status-pulse {
  0% {
    background-color: color-mix(in srgb, var(--pulse-color) 15%, transparent);
  }
  100% {
    background-color: transparent;
  }
}
</style>
