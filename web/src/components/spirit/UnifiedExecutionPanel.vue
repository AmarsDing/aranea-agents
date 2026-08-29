<template>
  <div class="unified-execution-panel">
    <!-- Section 1: Task Breakdown -->
    <div class="uep-section">
      <div
        class="uep-section__header"
        role="button"
        tabindex="0"
        @click="taskBreakdownOpen = !taskBreakdownOpen"
        @keydown.enter="taskBreakdownOpen = !taskBreakdownOpen"
      >
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

    <!-- Section 2: Dependencies (DAG flow with arrows) -->
    <div class="uep-section">
      <div
        class="uep-section__header"
        role="button"
        tabindex="0"
        @click="dependenciesOpen = !dependenciesOpen"
        @keydown.enter="dependenciesOpen = !dependenciesOpen"
      >
        <q-icon name="account_tree" size="16px" class="uep-section__icon" />
        <span class="uep-section__title">{{ t('chat.execution.dependencies') }}</span>
        <q-icon :name="dependenciesOpen ? 'expand_less' : 'expand_more'" size="16px" class="uep-section__arrow" />
      </div>
      <div v-if="dependenciesOpen" class="uep-section__body">
        <template v-if="hasDependencies">
          <div class="uep-dag-layers">
            <div v-for="(layer, layerIdx) in dagLayers" :key="layerIdx" class="uep-dag-layer">
              <div class="uep-dag-layer__nodes row items-center q-gutter-sm">
                <span
                  v-for="node in layer.nodes"
                  :key="node.id"
                  class="uep-dag-node"
                  :class="`uep-dag-node--${node.state}`"
                >
                  <span class="uep-dag-node__dot" :class="`uep-dag-node__dot--${node.state}`" />
                  {{ node.name }}
                </span>
              </div>
              <div v-if="layerIdx < dagLayers.length - 1" class="uep-dag-layer__arrow row justify-center">
                <span class="uep-dag-arrow">→</span>
              </div>
            </div>
          </div>
        </template>
        <div v-else class="uep-empty">{{ t('chat.execution.noDependencies') }}</div>
      </div>
    </div>

    <!-- Divider -->
    <div class="uep-divider" />

    <!-- Section 3: Team Progress (with rich headers) -->
    <div class="uep-section">
      <div
        class="uep-section__header"
        role="button"
        tabindex="0"
        @click="teamProgressOpen = !teamProgressOpen"
        @keydown.enter="teamProgressOpen = !teamProgressOpen"
      >
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
                <div class="uep-team-item__avatar">{{ teamInitial(team) }}</div>
                <span class="uep-team-item__name">{{ team.teamName }}</span>
                <span
                  class="uep-team-item__status-badge"
                  :class="`uep-team-item__status-badge--${teamStatusColor(team)}`"
                >
                  <span v-if="team.status === 'running'" class="uep-pulse-dot" />
                  <q-icon :name="teamStatusIcon(team)" size="11px" class="q-mr-xs" />
                  {{ teamStatusText(team) }}
                </span>
                <div class="uep-team-item__bar">
                  <div class="uep-team-item__bar-fill" :style="{ width: `${team.progressPct}%` }" />
                </div>
                <span class="uep-team-item__duration">{{ formatDuration(team.durationMs) }}</span>
                <template v-if="team.status === 'interrupted'">
                  <button
                    class="uep-team-item__action uep-team-item__action--resume"
                    @click.stop="$emit('resume-team', team.id)"
                  >
                    {{ t('spirit.resume') }}
                  </button>
                  <button
                    class="uep-team-item__action uep-team-item__action--cancel"
                    @click.stop="$emit('cancel-team', team.id)"
                  >
                    {{ t('spirit.cancel') }}
                  </button>
                </template>
                <q-icon
                  :name="isTeamExpanded(team.id) ? 'expand_less' : 'expand_more'"
                  size="14px"
                  class="uep-section__arrow"
                />
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
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SpiritTeam, TaskRow, DagFlowNode } from '../../features/spirit/types';
import type { TaskNode } from '../../features/spirit/types';
import type { PlanEntry } from '../../features/chat/agentTreeTypes';
import {
  spiritTeamStatusToLabel,
  STATUS_LABEL_CONFIG,
  formatDuration,
  planEntryStatusLabel,
  nameInitial,
} from '../../features/spirit/spiritUi';
import { kahnTopoLayers } from '../../features/spirit/lib/dagTopoSort';
import type { AgentNodeStatusLabel } from '../../features/spirit/spiritUi';
import TeamProgressCard from './TeamProgressCard.vue';

const { t } = useI18n();

const props = defineProps<{
  teams: SpiritTeam[];
  taskNodes?: TaskNode[];
  planEntries?: PlanEntry[];
}>();

defineEmits<{
  teamClick: [teamId: string];
  'resume-team': [teamId: string];
  'cancel-team': [teamId: string];
}>();

// ── Collapsible state ──
const taskBreakdownOpen = ref(true);
const dependenciesOpen = ref(true);
const teamProgressOpen = ref(true);

// ── Team expand/collapse state ──
const expandedTeamIds = ref<Set<string>>(new Set());

// Running/pending teams default expanded; completed/interrupted/cancelled default collapsed.
// Re-evaluate when teams list changes to handle new teams appearing.
watch(
  () => props.teams,
  (teams) => {
    const next = new Set(expandedTeamIds.value);
    for (const team of teams) {
      if ((team.status === 'running' || team.status === 'pending') && !next.has(team.id)) {
        next.add(team.id);
      }
    }
    expandedTeamIds.value = next;
  },
  { immediate: true },
);

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

// ── Team header helpers ──
function teamInitial(team: SpiritTeam): string {
  return nameInitial(team.teamName);
}

function teamStatusLabelKey(status: SpiritTeam['status']): AgentNodeStatusLabel {
  return spiritTeamStatusToLabel(status);
}

function teamStatusColor(team: SpiritTeam): string {
  return STATUS_LABEL_CONFIG[teamStatusLabelKey(team.status)]?.dotColor ?? 'grey';
}

function teamStatusText(team: SpiritTeam): string {
  return STATUS_LABEL_CONFIG[teamStatusLabelKey(team.status)]?.text ?? team.status;
}

function teamStatusIcon(team: SpiritTeam): string {
  return STATUS_LABEL_CONFIG[teamStatusLabelKey(team.status)]?.icon ?? 'circle';
}

// ── Section 1: Task Breakdown rows ──
const taskRows = computed<TaskRow[]>(() => {
  if (props.planEntries && props.planEntries.length > 0) {
    return props.planEntries.map((pe) => ({
      id: pe.id,
      taskName: pe.task,
      teamLabel: pe.agentName,
      isRunning: pe.status === 'running',
      statusText: t(planEntryStatusLabel(pe.status)),
    }));
  }

  if (props.taskNodes && props.taskNodes.length > 0) {
    const teamByDagNode = new Map<string, SpiritTeam>();
    for (const tm of props.teams) {
      if (tm.dagNodeId) teamByDagNode.set(tm.dagNodeId, tm);
    }

    return props.taskNodes.map((tn) => {
      const team = teamByDagNode.get(tn.id);
      return {
        id: tn.id,
        taskName: tn.taskName,
        teamLabel: team?.teamName ?? null,
        isRunning: team?.status === 'running',
        statusText: team ? teamStatusText(team) : t('chat.execution.statusPending'),
      };
    });
  }

  return [];
});

// ── Section 2: Dependencies (DAG flow) ──
const hasDependencies = computed(() => {
  if (!props.taskNodes || props.taskNodes.length === 0) return false;
  return props.taskNodes.some((tn) => tn.dependsOn.length > 0);
});

interface DagLayer {
  nodes: DagFlowNode[];
}

const dagLayers = computed<DagLayer[]>(() => {
  if (!props.taskNodes || props.taskNodes.length === 0) return [];

  const nodeMap = new Map<string, TaskNode>();
  for (const tn of props.taskNodes) {
    nodeMap.set(tn.id, tn);
  }

  const teamByDagNode = new Map<string, SpiritTeam>();
  for (const tm of props.teams) {
    if (tm.dagNodeId) teamByDagNode.set(tm.dagNodeId, tm);
  }

  // Kahn's algorithm: topological sort with cycle detection
  const depths = kahnTopoLayers(props.taskNodes);

  // Group by depth
  const maxDepth = Math.max(...Array.from(depths.values()), 0);
  const layers: DagLayer[] = [];

  for (let d = 0; d <= maxDepth; d++) {
    const layerNodes = props.taskNodes
      .filter((tn) => depths.get(tn.id) === d)
      .map((tn) => {
        const team = teamByDagNode.get(tn.id);
        let state: DagFlowNode['state'] = 'waiting';
        if (team) {
          // 修复：检查是否有成员失败，如果有则显示为 partial_failure（黄色警告）
          const hasFailedMember = team.members?.some((m) => m.status === 'failed');

          if (team.status === 'partial_failure') {
            // 后端持久态：交付物门通过但 ≥1 成员失败
            state = 'partial_failure';
          } else if (team.status === 'completed' || team.status === 'archived') {
            // 旧数据回退：completed 但有成员失败时也显示 partial_failure
            state = hasFailedMember ? 'partial_failure' : 'done';
          } else if (team.status === 'running' || team.status === 'pending') {
            state = 'running';
          } else if (team.status === 'failed' || team.status === 'cancelled') {
            state = 'failed';
          } else if (team.status === 'interrupted') {
            state = 'interrupted';
          }
        }
        return {
          id: tn.id,
          name: tn.taskName,
          state,
          depLabels: tn.dependsOn.map((depId) => nodeMap.get(depId)?.taskName ?? depId),
        };
      });
    if (layerNodes.length > 0) {
      layers.push({ nodes: layerNodes });
    }
  }

  return layers;
});
</script>

<style scoped lang="sass">
.unified-execution-panel
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
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
    font-size: var(--text-xs)
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
  border-top: 1px solid var(--glass-border)

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
    font-size: var(--text-xs)
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
    font-size: var(--text-xs)
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
    font-size: var(--text-xs)
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

// ── DAG Layers (topological with branch/merge) ──
.uep-dag-layers
  display: flex
  flex-direction: column
  gap: 2px

.uep-dag-layer
  &__nodes
    justify-content: center

  &__arrow
    padding: 1px 0
    color: var(--color-text-tertiary)

.uep-dag-arrow
  font-size: 14px
  color: var(--color-text-tertiary)
  line-height: 1

.uep-dag-node
  display: inline-flex
  align-items: center
  gap: 4px
  padding: 3px 8px
  border-radius: 4px
  font-size: var(--text-xs)
  background: color-mix(in srgb, var(--color-primary) 10%, transparent)
  color: var(--color-text-secondary)

  &--done
    opacity: 0.5

  &--running
    background: color-mix(in srgb, var(--color-primary) 15%, transparent)
    color: var(--color-primary)

  &--waiting
    // default

  &--failed
    background: color-mix(in srgb, var(--color-danger) 10%, transparent)
    color: var(--color-danger)

  &--interrupted
    background: color-mix(in srgb, var(--color-warning) 10%, transparent)
    color: var(--color-warning)

  &--partial_failure
    background: color-mix(in srgb, var(--color-warning) 12%, transparent)
    color: var(--color-warning)

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

    &--failed
      background: var(--color-danger)

    &--interrupted
      background: var(--color-warning)

    &--partial_failure
      background: var(--color-warning)

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
    padding: 6px 8px
    border-radius: 6px
    cursor: pointer
    user-select: none
    &:hover
      background: color-mix(in srgb, var(--color-primary) 4%, transparent)

  &__avatar
    width: 20px
    height: 20px
    border-radius: 50%
    background: var(--glass-elevated)
    display: flex
    align-items: center
    justify-content: center
    font-size: 9px
    font-weight: 600
    color: var(--color-text-secondary)
    flex-shrink: 0

  &__name
    font-size: var(--text-xs)
    font-weight: 500
    color: var(--color-text-primary)
    flex: 1
    min-width: 0
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__status-badge
    padding: 1px 6px
    border-radius: 4px
    font-size: var(--text-xs)
    font-weight: 500
    display: inline-flex
    align-items: center
    gap: 4px
    flex-shrink: 0

    &--blue
      background: color-mix(in srgb, var(--color-accent) 15%, transparent)
      color: var(--color-accent)

    &--green
      background: color-mix(in srgb, var(--color-success) 12%, transparent)
      color: var(--color-success)

    &--red
      background: color-mix(in srgb, var(--color-danger) 12%, transparent)
      color: var(--color-danger)

    &--orange
      background: color-mix(in srgb, var(--color-warning) 12%, transparent)
      color: var(--color-warning)

    &--grey
      background: color-mix(in srgb, var(--color-text-tertiary) 10%, transparent)
      color: var(--color-text-tertiary)

  &__bar
    width: 50px
    height: 3px
    background: var(--glass-border)
    border-radius: 2px
    overflow: hidden
    flex-shrink: 0

  &__bar-fill
    height: 100%
    border-radius: 2px
    background: var(--color-primary)
    transition: width 0.3s

  &__duration
    font-size: var(--text-xs)
    color: var(--color-text-tertiary)
    flex-shrink: 0

  &__action
    border: none
    border-radius: 3px
    padding: 1px 5px
    font-size: var(--text-xs)
    cursor: pointer
    flex-shrink: 0
    transition: opacity 0.15s ease

    &:hover
      opacity: 0.85

    &--resume
      background: var(--color-accent)
      color: var(--color-on-accent)

    &--cancel
      background: var(--glass-elevated)
      border: 1px solid var(--glass-border)
      color: var(--color-text-secondary)

  &__card
    margin-top: 4px
</style>
