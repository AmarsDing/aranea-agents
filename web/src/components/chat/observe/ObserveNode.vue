<template>
  <div
    :class="[
      'observe-node',
      `observe-node--${data.status}`,
      { 'observe-node--entry': data.isEntry, 'observe-node--exit': data.isExit },
    ]"
  >
    <!-- Entry/Exit markers -->
    <div v-if="data.isEntry" class="observe-node__entry-marker" />
    <div v-if="data.isExit" class="observe-node__exit-marker" />

    <!-- Header: status dot + label + status badge -->
    <header class="observe-node__header">
      <span :class="['observe-node__dot', `observe-node__dot--${data.status}`]" />
      <span class="observe-node__name" :title="data.label">{{ data.label }}</span>
      <ObserveStatusBadge :status="data.status" />
    </header>

    <!-- Description (if available) -->
    <div v-if="data.description" class="observe-node__description" :title="data.description">
      {{ data.description }}
    </div>

    <!-- Members: single = avatar + name; team = avatar stack + count -->
    <div v-if="data.members?.length" class="observe-node__members">
      <div class="observe-node__avatars">
        <span v-for="m in displayMembers" :key="m.agentKey" class="observe-node__avatar" :title="m.agentName">
          <img v-if="m.avatarUrl" :src="m.avatarUrl" :alt="m.agentName" class="observe-node__avatar-img" />
          <template v-else>{{ memberInitial(m.agentName) }}</template>
          <span :class="['observe-node__member-dot', `observe-node__member-dot--${m.status}`]" />
        </span>
        <span v-if="extraMemberCount > 0" class="observe-node__avatar observe-node__avatar--more">
          +{{ extraMemberCount }}
        </span>
      </div>
      <span class="observe-node__member-names">{{ memberNamesLabel }}</span>
      <!-- Team badge: show member count for multi-member nodes -->
      <span v-if="isTeamNode" class="observe-node__team-badge" :title="`${data.members!.length} members`">
        {{ data.members!.length }}
      </span>
    </div>

    <!-- Progress bar (running only) -->
    <div v-if="data.status === 'running'" class="observe-node__progress">
      <q-linear-progress :value="progressValue" color="warning" size="3px" rounded class="observe-node__progress-bar" />
      <span v-if="progressLabel" class="observe-node__progress-label">{{ progressLabel }}</span>
    </div>

    <!-- Media preview (partial while running, full when completed) -->
    <NodeMediaPreview v-if="data.mediaOutput?.length" :artifacts="data.mediaOutput" @preview="onMediaPreview" />

    <!-- Text output summary (completed only) -->
    <div
      v-if="data.status === 'completed' && data.textOutput"
      class="observe-node__text-output"
      :title="data.textOutput"
    >
      <q-icon name="description" size="12px" />
      <span class="observe-node__text-output-text">{{ data.textOutput }}</span>
    </div>

    <!-- Current action (running only) -->
    <div v-if="data.status === 'running' && currentAction" class="observe-node__action">
      <q-icon name="bolt" size="12px" />
      <span class="observe-node__action-text">{{ currentAction }}</span>
    </div>

    <!-- Error (failed only) -->
    <div v-if="data.status === 'failed' && data.error" class="observe-node__error" :title="data.error">
      <q-icon name="error" size="12px" />
      <span class="observe-node__error-text">{{ data.error }}</span>
    </div>

    <!-- Footer: duration (running = elapsed, completed = total) -->
    <div v-if="durationLabel" class="observe-node__footer">
      <q-icon name="schedule" size="11px" />
      <span>{{ durationLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue';
import type { GraphNodeStatus } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { formatDuration } from '../../../features/spirit/spiritUi';
import ObserveStatusBadge from './ObserveStatusBadge.vue';
import NodeMediaPreview from './NodeMediaPreview.vue';

interface NodeMember {
  agentKey: string;
  agentName: string;
  avatarUrl?: string;
  status: string;
  currentAction?: string;
}

interface ObserveNodeData {
  label: string;
  dagNodeId: string;
  teamStageId: string;
  status: GraphNodeStatus;
  dependsOn: string[];
  members?: NodeMember[];
  activeMemberCount?: number;
  mediaOutput: MediaArtifact[];
  progress?: { value: number; max: number; label?: string };
  durationMs?: number;
  error?: string;
  description?: string;
  textOutput?: string;
  isEntry?: boolean;
  isExit?: boolean;
}

const props = defineProps<{ data: ObserveNodeData }>();

// Custom-node emits do not bubble through Vue Flow; the canvas bridges
// media preview via provide/inject.
const previewBridge = inject<(art: MediaArtifact) => void>('observe-media-preview', () => {});
function onMediaPreview(art: MediaArtifact) {
  previewBridge(art);
}

const MAX_VISIBLE_MEMBERS = 3;

const isTeamNode = computed(() => (props.data.members?.length || 0) > 1);

const displayMembers = computed(() => (props.data.members || []).slice(0, MAX_VISIBLE_MEMBERS));
const extraMemberCount = computed(() => Math.max(0, (props.data.members?.length || 0) - MAX_VISIBLE_MEMBERS));

const memberNamesLabel = computed(() => {
  const members = props.data.members || [];
  if (members.length === 0) return '';
  if (members.length === 1) return members[0].agentName;
  return members.map((m) => m.agentName).join('、');
});

function memberInitial(name: string): string {
  return (name || '?').charAt(0).toUpperCase();
}

// First running member's current action.
const currentAction = computed(() => {
  const members = props.data.members || [];
  const running = members.find((m) => m.status === 'running' && m.currentAction);
  return running?.currentAction || '';
});

const progressValue = computed(() => {
  if (props.data.progress) {
    return props.data.progress.value / props.data.progress.max;
  }
  return 0.5; // indeterminate-like
});

const progressLabel = computed(() => {
  if (props.data.progress?.label) {
    return props.data.progress.label;
  }
  if (props.data.progress) {
    return `${Math.round((props.data.progress.value / props.data.progress.max) * 100)}%`;
  }
  return '';
});

const durationLabel = computed(() => formatDuration(props.data.durationMs));
</script>

<style scoped lang="sass">
.observe-node
  position: relative
  border: 2px solid var(--color-border-soft)
  border-radius: 10px
  background: var(--color-surface-solid)
  min-width: 200px
  max-width: 260px
  padding: 10px
  font-size: 12px
  color: var(--color-text-primary)
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08)
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease, border-width 0.15s ease
  animation: observe-node-enter 0.25s ease

  &:hover
    border-width: 3px
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.14)

  &--running
    border-color: var(--color-warning)
    animation: observe-node-enter 0.25s ease, observe-pulse 1.5s infinite

  &--completed
    border-color: var(--color-success)
    background: color-mix(in srgb, var(--color-success) 5%, var(--color-surface-solid))

  &--failed
    border-color: var(--color-danger)
    background: color-mix(in srgb, var(--color-danger) 5%, var(--color-surface-solid))

  &--interrupted
    border-color: var(--color-warning)
    opacity: 0.75

// ── Header ──
.observe-node__header
  display: flex
  align-items: center
  gap: 6px

.observe-node__dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

  &--pending
    background: var(--color-text-tertiary)

  &--running
    background: var(--color-warning)

  &--completed
    background: var(--color-success)

  &--failed
    background: var(--color-danger)

  &--interrupted
    background: var(--color-warning)

.observe-node__name
  font-size: 13px
  font-weight: 600
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

// ── Members ──
.observe-node__members
  display: flex
  align-items: center
  gap: 6px
  margin-top: 8px

.observe-node__avatars
  display: flex
  flex-shrink: 0

.observe-node__avatar
  position: relative
  width: 24px
  height: 24px
  border-radius: 50%
  background: var(--q-primary)
  color: var(--color-on-accent)
  display: flex
  align-items: center
  justify-content: center
  font-size: 11px
  font-weight: 600
  border: 2px solid var(--color-surface-solid)
  overflow: visible

  & + &
    margin-left: -8px

  &--more
    background: var(--color-surface-soft)
    color: var(--color-text-secondary)
    font-size: 10px

.observe-node__avatar-img
  width: 100%
  height: 100%
  border-radius: 50%
  object-fit: cover

.observe-node__member-dot
  position: absolute
  right: -1px
  bottom: -1px
  width: 8px
  height: 8px
  border-radius: 50%
  border: 1.5px solid var(--color-surface-solid)

  &--completed
    background: var(--color-success)

  &--running
    background: var(--color-warning)

  &--failed
    background: var(--color-danger)

  &--pending,
  &--skipped
    background: var(--color-text-tertiary)

.observe-node__member-names
  font-size: 11px
  font-weight: 500
  color: var(--color-text-primary)
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.observe-node__team-badge
  flex-shrink: 0
  margin-left: 4px
  padding: 1px 5px
  border-radius: 8px
  background: var(--q-primary)
  color: var(--color-on-accent)
  font-size: 9px
  font-weight: 600
  line-height: 1.4

// ── Progress ──
.observe-node__progress
  margin-top: 8px
  display: flex
  align-items: center
  gap: 6px

.observe-node__progress-bar
  flex: 1
  animation: observe-progress-breathe 1.6s ease-in-out infinite

.observe-node__progress-label
  font-size: 10px
  color: var(--color-text-tertiary)
  flex-shrink: 0

// ── Current action ──
.observe-node__action
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  color: var(--color-text-tertiary)
  font-size: 11px

.observe-node__action-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

// ── Error ──
.observe-node__error
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  color: var(--color-danger)
  font-size: 11px

.observe-node__error-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

// ── Footer ──
.observe-node__footer
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  font-size: 10px
  color: var(--color-text-tertiary)

// ── Entry/Exit markers ──
.observe-node__entry-marker
  position: absolute
  left: 0
  top: 50%
  transform: translateY(-50%)
  width: 4px
  height: 60%
  background: var(--color-success)
  border-radius: 2px 0 0 2px

.observe-node__exit-marker
  position: absolute
  right: 0
  top: 50%
  transform: translateY(-50%)
  width: 4px
  height: 60%
  background: var(--color-accent)
  border-radius: 0 2px 2px 0

// ── Description ──
.observe-node__description
  font-size: 11px
  color: var(--color-text-secondary)
  margin-top: 6px
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

// ── Text Output ──
.observe-node__text-output
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  color: var(--color-text-secondary)
  font-size: 11px

.observe-node__text-output-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

// ── Animations ──
@keyframes observe-pulse
  0%, 100%
    box-shadow: 0 0 0 0 rgba(255, 152, 0, 0.4)
  50%
    box-shadow: 0 0 0 8px rgba(255, 152, 0, 0)

@keyframes observe-progress-breathe
  0%, 100%
    opacity: 1
  50%
    opacity: 0.7

@keyframes observe-node-enter
  from
    opacity: 0
    transform: scale(0.9)
  to
    opacity: 1
    transform: scale(1)
</style>
