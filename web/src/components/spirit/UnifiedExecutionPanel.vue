<template>
  <div class="unified-execution-panel">
    <!-- Section 1: Task Breakdown -->
    <div class="uep-section">
      <div class="uep-section__header" role="button" tabindex="0" @click="taskBreakdownOpen = !taskBreakdownOpen" @keydown.enter="taskBreakdownOpen = !taskBreakdownOpen">
        <q-icon name="format_list_numbered" size="16px" class="uep-section__icon" />
        <span class="uep-section__title">{{ t('chat.execution.taskBreakdown') }}</span>
        <span v-if="taskRows.length > 0" class="uep-section__badge">{{ taskRows.length }}</span>
        <q-icon :name="taskBreakdownOpen ? 'expand_less' : 'expand_more'" size="16px" class="uep-section__arrow" />
      </div>
      <div v-if="taskBreakdownOpen" class="uep-section__body">
        <template v-if="taskRows.length > 0">
          <div v-for="(row, idx) in taskRows" :key="row.id" class="uep-task-row">
            <span class="uep-task-row__num">{{ idx + 1 }}</span>
            <span class="uep-task-row__name">{{ row.taskName }}</span>
            <span v-if="row.teamLabel" class="uep-task-row__team">{{ row.teamLabel }}</span>
            <span class="uep-task-row__status" :class="{ 'uep-task-row__status--running': row.isRunning }">
              <span v-if="row.isRunning" class="uep-pulse-dot" />
              {{ row.statusText }}
            </span>
          </div>
        </template>
        <div v-else class="uep-empty">{{ t('chat.execution.noTasks') }}</div>
      </div>
    </div>

    <!-- Divider -->
    <div class="uep-divider" />

    <!-- Section 2: Dependencies -->
    <div class="uep-section">
      <div class="uep-section__header" role="button" tabindex="0" @click="dependenciesOpen = !dependenciesOpen" @keydown.enter="dependenciesOpen = !dependenciesOpen">
        <q-icon name="account_tree" size="16px" class="uep-section__icon" />
        <span class="uep-section__title">{{ t('chat.execution.dependencies') }}</span>
        <q-icon :name="dependenciesOpen ? 'expand_less' : 'expand_more'" size="16px" class="uep-section__arrow" />
      </div>
      <div v-if="dependenciesOpen" class="uep-section__body">
        <template v-if="hasDependencies">
          <div class="uep-dag-flow">
            <div v-for="node in dagFlowNodes" :key="node.id" class="uep-dag-node" :class="`uep-dag-node--${node.state}`">
              <span class="uep-dag-node__dot" :class="`uep-dag-node__dot--${node.state}`" />
              <span class="uep-dag-node__name">{{ node.name }}</span>
              <span v-if="node.depLabels.length > 0" class="uep-dag-node__deps">
                ← {{ node.depLabels.join(', ') }}
              </span>
            </div>
          </div>
        </template>
        <div v-else class="uep-empty">{{ t('chat.execution.noDependencies') }}</div>
      </div>
    </div>

    <!-- Divider -->
    <div class="uep-divider" />

    <!-- Section 3: Team Progress -->
    <div class="uep-section">
      <div class="uep-section__header" role="button" tabindex="0" @click="teamProgressOpen = !teamProgressOpen" @keydown.enter="teamProgressOpen = !teamProgressOpen">
        <q-icon name="groups" size="16px" class="uep-section__icon" />
        <span class="uep-section__title">{{ t('chat.execution.teamProgress') }}</span>
        <span v-if="teams.length > 0" class="uep-section__badge">{{ teams.length }}</span>
        <q-icon :name="teamProgressOpen ? 'expand_less' : 'expand_more'" size="16px" class="uep-section__arrow" />
      </div>
      <div v-if="teamProgressOpen" class="uep-section__body">
        <template v-if="teams.length > 0">
          <div class="uep-team-list">
            <div v-for="team in teams" :key="team.id" class="uep-team-item">
              <div
                class="uep-team-item__header"
                role="button"
                tabindex="0"
                @click="toggleTeamExpand(team.id)"
                @keydown.enter="toggleTeamExpand(team.id)"
              >
                <span class="uep-team-item__name">{{ team.teamName }}</span>
                <span class="uep-team-item__pct">{{ team.progressPct }}%</span>
                <q-icon :name="isTeamExpanded(team.id) ? 'expand_less' : 'expand_more'" size="14px" class="uep-section__arrow" />
              </div>
              <div v-if="isTeamExpanded(team.id)" class="uep-team-item__card">
                <TeamProgressCard :team="team" @click="$emit('teamClick', team.id)" />
              </div>
            </div>
          </div>
        </template>
        <div v-else class="uep-empty">{{ t('chat.execution.noTeams') }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritTeam, TaskNode } from '../../features/spirit/types';
import type { PlanEntry } from '../../features/chat/agentTreeTypes';
import { spiritTeamStatusToLabel, STATUS_LABEL_CONFIG } from '../../features/spirit/spiritUi';
import TeamProgressCard from './TeamProgressCard.vue';

const { t } = useI18n();

const props = defineProps<{
  teams: SpiritTeam[];
  taskNodes?: TaskNode[];
  planEntries?: PlanEntry[];
}>();

defineEmits<{
  teamClick: [teamId: string];
}>();

// ── Collapsible state ──
const taskBreakdownOpen = ref(true);
const dependenciesOpen = ref(true);
const teamProgressOpen = ref(true);

// ── Team expand/collapse state ──
const expandedTeamIds = ref<Set<string>>(new Set());

// Running teams default expanded, completed/interrupted default collapsed
function initTeamExpandState() {
  const ids = new Set<string>();
  for (const team of props.teams) {
    if (team.status === 'running' || team.status === 'pending') {
      ids.add(team.id);
    }
  }
  expandedTeamIds.value = ids;
}

// Initialize on creation
initTeamExpandState();

function toggleTeamExpand(teamId: string) {
  const next = new Set(expandedTeamIds.value);
  if (next.has(teamId)) {
    next.delete(teamId);
  } else {
    next.add(teamId);
  }
  expandedTeamIds.value = next;
}

function isTeamExpanded(teamId: string): boolean {
  return expandedTeamIds.value.has(teamId);
}

// ── Section 1: Task Breakdown rows ──
type TaskRow = {
  id: string;
  taskName: string;
  teamLabel: string | null;
  isRunning: boolean;
  statusText: string;
};

const taskRows = computed<TaskRow[]>(() => {
  // Prefer planEntries (from useAgentBlocks), fall back to taskNodes
  if (props.planEntries && props.planEntries.length > 0) {
    return props.planEntries.map((pe) => ({
      id: pe.id,
      taskName: pe.task,
      teamLabel: pe.agentName,
      isRunning: pe.status === 'running',
      statusText: planEntryStatusLabel(pe.status),
    }));
  }

  if (props.taskNodes && props.taskNodes.length > 0) {
    return props.taskNodes.map((tn) => {
      const team = props.teams.find((t) => t.dagNodeId === tn.id);
      return {
        id: tn.id,
        taskName: tn.taskName,
        teamLabel: team?.teamName ?? null,
        isRunning: team?.status === 'running',
        statusText: team ? teamStatusLabel(team.status) : t('chat.execution.statusPending'),
      };
    });
  }

  return [];
});

function planEntryStatusLabel(status: PlanEntry['status']): string {
  const labels: Record<string, string> = {
    pending: t('chat.execution.statusPending'),
    running: t('chat.execution.statusRunning'),
    completed: t('chat.execution.statusCompleted'),
    failed: t('chat.execution.statusFailed'),
  };
  return labels[status] ?? t('chat.execution.statusPending');
}

function teamStatusLabel(status: SpiritTeam['status']): string {
  const label = spiritTeamStatusToLabel(status);
  return STATUS_LABEL_CONFIG[label]?.text ?? t('chat.execution.statusPending');
}

// ── Section 2: Dependencies (DAG flow) ──
type DagFlowNode = {
  id: string;
  name: string;
  state: 'done' | 'running' | 'waiting';
  depLabels: string[];
};

const hasDependencies = computed(() => {
  if (!props.taskNodes || props.taskNodes.length === 0) return false;
  return props.taskNodes.some((tn) => tn.dependsOn.length > 0);
});

const dagFlowNodes = computed<DagFlowNode[]>(() => {
  if (!props.taskNodes || props.taskNodes.length === 0) return [];

  const nodeMap = new Map<string, TaskNode>();
  for (const tn of props.taskNodes) {
    nodeMap.set(tn.id, tn);
  }

  return props.taskNodes.map((tn) => {
    const team = props.teams.find((t) => t.dagNodeId === tn.id);
    let state: DagFlowNode['state'] = 'waiting';
    if (team) {
      if (team.status === 'completed' || team.status === 'archived') {
        state = 'done';
      } else if (team.status === 'running') {
        state = 'running';
      }
    }

    const depLabels = tn.dependsOn
      .map((depId) => {
        const depNode = nodeMap.get(depId);
        return depNode?.taskName ?? depId;
      });

    return {
      id: tn.id,
      name: tn.taskName,
      state,
      depLabels,
    };
  });
});
</script>

<style scoped lang="sass">
.unified-execution-panel
  background: var(--glass-surface, var(--color-bg-surface))
  border: 1px solid var(--glass-border, var(--color-border))
  border-radius: var(--radius)

// ── Section ──
.uep-section
  &__header
    display: flex
    align-items: center
    gap: 6px
    padding: 10px 12px
    cursor: pointer
    user-select: none
    &:hover
      background: color-mix(in srgb, var(--color-primary) 4%, transparent)

  &__icon
    color: var(--color-primary)
    flex-shrink: 0

  &__title
    font-size: var(--text-sm)
    font-weight: 600
    color: var(--color-text-primary)
    flex: 1

  &__badge
    font-size: 10px
    font-weight: 600
    line-height: 1
    min-width: 16px
    height: 16px
    display: inline-flex
    align-items: center
    justify-content: center
    border-radius: 8px
    background: color-mix(in srgb, var(--color-primary) 12%, transparent)
    color: var(--color-primary)
    padding: 0 4px

  &__arrow
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__body
    padding: 0 12px 10px 12px

// ── Divider ──
.uep-divider
  border-top: 1px solid var(--glass-border, var(--color-border))

// ── Empty placeholder ──
.uep-empty
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  text-align: center
  padding: 12px 0

// ── Task Row ──
.uep-task-row
  display: flex
  align-items: center
  gap: 8px
  padding: 4px 0

  &__num
    width: 18px
    height: 18px
    border-radius: 50%
    background: color-mix(in srgb, var(--color-primary) 12%, transparent)
    color: var(--color-primary)
    font-size: 10px
    font-weight: 600
    display: inline-flex
    align-items: center
    justify-content: center
    flex-shrink: 0

  &__name
    font-size: var(--text-xs)
    color: var(--color-text-primary)
    flex: 1
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__team
    font-size: 10px
    background: var(--glass-surface)
    padding: 1px 6px
    border-radius: 3px
    color: var(--color-text-secondary)
    flex-shrink: 0
    max-width: 80px
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__status
    font-size: 10px
    color: var(--color-text-tertiary)
    flex-shrink: 0
    display: inline-flex
    align-items: center
    gap: 4px

    &--running
      color: var(--color-primary)

// ── Pulse dot ──
.uep-pulse-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-primary)
  animation: uep-pulse 1.5s ease-in-out infinite

@keyframes uep-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

// ── DAG Flow ──
.uep-dag-flow
  display: flex
  flex-direction: column
  gap: 4px

.uep-dag-node
  display: flex
  align-items: center
  gap: 6px
  padding: 3px 6px
  border-radius: 4px
  font-size: var(--text-xs)

  &--done
    opacity: 0.5

  &--running
    background: color-mix(in srgb, var(--color-primary) 6%, transparent)

  &--waiting
    // default

  &__dot
    width: 6px
    height: 6px
    border-radius: 50%
    flex-shrink: 0

    &--done
      background: var(--color-primary)
      opacity: 0.5

    &--running
      background: var(--color-primary)
      animation: uep-pulse 1.5s ease-in-out infinite

    &--waiting
      background: var(--color-text-tertiary)
      opacity: 0.4

  &__name
    font-weight: 500
    color: var(--color-text-primary)

  &__deps
    font-size: 10px
    color: var(--color-text-tertiary)
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis

// ── Team Progress List ──
.uep-team-list
  display: flex
  flex-direction: column
  gap: 4px

.uep-team-item
  &__header
    display: flex
    align-items: center
    gap: 6px
    padding: 4px 6px
    border-radius: 4px
    cursor: pointer
    &:hover
      background: color-mix(in srgb, var(--color-primary) 4%, transparent)

  &__name
    font-size: var(--text-xs)
    font-weight: 500
    color: var(--color-text-primary)
    flex: 1
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__pct
    font-size: 10px
    color: var(--color-primary)
    font-weight: 600
    flex-shrink: 0

  &__card
    margin-top: 4px
</style>
