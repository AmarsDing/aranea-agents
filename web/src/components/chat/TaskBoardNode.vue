<template>
  <div
    class="tb-node"
    :class="[`tb-node--${node.kind}`, { 'tb-node--collapsed': localCollapsed }]"
    :style="{ '--node-color': nodeColor }"
  >
    <!-- Node header -->
    <div class="tb-node__header" @click="toggleCollapse">
      <span v-if="nodeIconText" class="tb-node__icon-text" :style="{ color: 'var(--node-color)' }">{{ nodeIconText }}</span>
      <q-icon v-else :name="nodeIcon" size="16px" :style="{ color: 'var(--node-color)' }" />
      <span class="tb-node__label">{{ nodeLabel }}</span>

      <!-- Thinking collapsed summary -->
      <template v-if="node.kind === 'thinking' && localCollapsed && thinkingSummary">
        <span class="tb-node__thinking-summary">🧠 {{ thinkingSummary }}</span>
      </template>

      <!-- Action collapsed summary -->
      <template v-if="node.kind === 'action' && localCollapsed">
        <span class="tb-node__action-summary">
          {{ actionSummary }}
        </span>
      </template>

      <!-- Duration -->
      <span v-if="formattedDuration" class="tb-node__duration">{{ formattedDuration }}</span>

      <!-- Streaming indicator for thinking -->
      <span v-if="node.kind === 'thinking' && isThinkingStreaming" class="tb-node__pulse" />

      <!-- Expand/collapse toggle -->
      <q-icon
        v-if="isCollapsible"
        class="tb-node__toggle"
        :class="{ 'tb-node__toggle--expanded': !localCollapsed }"
        name="expand_more"
        size="14px"
      />
    </div>

    <!-- Collapsible content -->
    <transition name="tb-collapse">
      <div v-show="!localCollapsed" class="tb-node__body">
        <!-- Task -->
        <div v-if="node.kind === 'task'" class="tb-node__content">
          {{ node.content }}
        </div>

        <!-- Thinking -->
        <div v-else-if="node.kind === 'thinking'" class="tb-node__content tb-node__content--thinking">
          {{ node.content }}
          <span v-if="isThinkingStreaming" class="tb-node__cursor" />
        </div>

        <!-- Action -->
        <div v-else-if="node.kind === 'action'" class="tb-node__content tb-node__content--action">
          <div v-if="node.arguments" class="tb-node__args">
            <div class="tb-node__args-label">{{ t('chat.toolArgs') }}</div>
            <pre>{{ node.arguments }}</pre>
          </div>
          <div v-if="node.result" class="tb-node__result">
            <div class="tb-node__result-label">{{ t('chat.toolResult') }}</div>
            <pre>{{ node.result }}</pre>
          </div>
        </div>

        <!-- Reply -->
        <div v-else-if="node.kind === 'reply'" class="tb-node__content tb-node__content--reply">
          {{ node.content }}
        </div>

        <!-- Sub-task board (recursive) -->
        <div v-else-if="node.kind === 'sub_task_board'" class="tb-node__sub-board">
          <TaskBoard :entries="node.children ?? []" :depth="depth + 1" />
        </div>

        <!-- End -->
        <div v-else-if="node.kind === 'end'" class="tb-node__content tb-node__content--end">
          {{ t('chat.taskBoard.complete') }}
        </div>

        <!-- Error -->
        <div v-else-if="node.kind === 'error'" class="tb-node__content tb-node__content--error">
          {{ node.content }}
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TaskBoardNodeData } from '../../features/chat/agentTreeTypes';
import { TASK_BOARD_NODE_ICON_TEXT } from '../../features/chat/agentTreeTypes';
import { formatDuration, truncateThinkingSummary } from '../../features/chat/agentTreeUtils';
import TaskBoard from './TaskBoard.vue';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    node: TaskBoardNodeData;
    depth?: number;
  }>(),
  { depth: 0 },
);

// ── Collapse state ──

const defaultCollapsed = computed(() => {
  if (props.node.kind === 'thinking') return true;
  if (props.node.kind === 'action') return true;
  return false;
});

const localCollapsed = ref(defaultCollapsed.value);

// Auto-collapse thinking when streaming finishes
watch(
  () => props.node.streaming,
  (streaming, oldStreaming) => {
    if (oldStreaming === true && streaming === false && props.node.kind === 'thinking') {
      localCollapsed.value = true;
    }
  },
);

// Auto-collapse action when toolStatus transitions from running to terminal state
watch(
  () => props.node.toolStatus,
  (newStatus, oldStatus) => {
    if (
      props.node.kind === 'action' &&
      oldStatus === 'running' &&
      (newStatus === 'success' || newStatus === 'failed' || newStatus === 'cancelled')
    ) {
      localCollapsed.value = true;
    }
  },
);

const isCollapsible = computed(() => {
  return props.node.kind !== 'end' && props.node.kind !== 'error' && props.node.kind !== 'task' && props.node.kind !== 'reply';
});

function toggleCollapse() {
  if (!isCollapsible.value) return;
  localCollapsed.value = !localCollapsed.value;
}

// ── Computed ──

const isThinkingStreaming = computed(() => props.node.kind === 'thinking' && props.node.streaming === true);

const nodeColor = computed(() => {
  switch (props.node.kind) {
    case 'task':
      return 'var(--color-accent)';
    case 'thinking':
      return 'var(--color-info)';
    case 'action':
      return 'var(--color-warning)';
    case 'reply':
      return 'var(--color-success)';
    case 'sub_task_board':
      return 'var(--color-accent)';
    case 'end':
      return 'var(--color-text-tertiary, grey)';
    case 'error':
      return 'var(--color-negative, var(--color-danger))';
    default:
      return 'var(--color-text-tertiary, grey)';
  }
});

const nodeIconText = computed(() => TASK_BOARD_NODE_ICON_TEXT[props.node.kind] ?? '');

const nodeIcon = computed(() => {
  switch (props.node.kind) {
    case 'task':
      return 'assignment';
    case 'thinking':
      return 'psychology';
    case 'action':
      return 'build';
    case 'reply':
      return 'chat';
    case 'sub_task_board':
      return 'account_tree';
    case 'end':
      return 'flag';
    case 'error':
      return 'error';
    default:
      return 'circle';
  }
});

const nodeLabel = computed(() => {
  switch (props.node.kind) {
    case 'task':
      return t('chat.taskBoard.task');
    case 'thinking':
      return t('chat.taskBoard.thinking');
    case 'action':
      return t('chat.taskBoard.action');
    case 'reply':
      return t('chat.taskBoard.reply');
    case 'sub_task_board':
      return t('chat.taskBoard.subTasks');
    case 'end':
      return t('chat.taskBoard.complete');
    case 'error':
      return t('chat.taskBoard.error');
    default:
      return '';
  }
});

const formattedDuration = computed(() => {
  const ms = props.node.durationMs;
  return ms != null ? formatDuration(ms) : '';
});

const thinkingSummary = computed(() => {
  if (props.node.kind !== 'thinking' || !props.node.content) return '';
  return truncateThinkingSummary(props.node.content);
});

const actionSummary = computed(() => {
  if (props.node.kind !== 'action') return '';
  const name = props.node.toolName || '';
  const status = props.node.toolStatus;
  const duration = formattedDuration.value;
  const parts: string[] = [name];
  if (status) parts.push(actionStatusDot(status));
  if (duration) parts.push(duration);
  return parts.filter(Boolean).join(' · ');
});

function actionStatusDot(status: string): string {
  switch (status) {
    case 'running':
      return '⏳';
    case 'success':
      return '✓';
    case 'failed':
    case 'cancelled':
      return '✗';
    case 'blocked':
      return '⏸';
    default:
      return '';
  }
}
</script>

<style scoped lang="sass">
.tb-node
  border-left: 3px solid var(--node-color)
  border-radius: 0 8px 8px 0
  margin-bottom: 8px
  background: color-mix(in srgb, var(--node-color) 4%, transparent)
  transition: border-color 0.2s, background 0.2s
  animation: tb-fade-in 0.25s ease

  &:hover
    background: color-mix(in srgb, var(--node-color) 7%, transparent)

.tb-node--error
  border-left-color: var(--color-negative, var(--color-danger))
  background: color-mix(in srgb, var(--color-negative, var(--color-danger)) 6%, transparent)

  .tb-node__content
    color: var(--color-negative, var(--color-danger))

.tb-node__header
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 10px
  cursor: pointer
  user-select: none
  transition: background 0.12s

  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)

.tb-node__label
  font-size: 12px
  font-weight: 600
  text-transform: uppercase
  letter-spacing: 0.3px
  color: var(--node-color)

.tb-node__icon-text
  font-size: 14px
  line-height: 1
  flex-shrink: 0

.tb-node__thinking-summary
  font-size: 11px
  color: var(--color-text-secondary)
  flex: 1
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  font-style: italic

.tb-node__action-summary
  font-size: 11px
  color: var(--color-text-secondary)
  flex: 1
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.tb-node__duration
  font-size: 10px
  color: var(--color-text-tertiary)

.tb-node__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--node-color)
  animation: tb-pulse 1s infinite
  margin-left: 2px

.tb-node__toggle
  margin-left: auto
  color: var(--color-text-tertiary)
  transition: transform 0.2s

.tb-node__toggle--expanded
  transform: rotate(180deg)

.tb-node__body
  padding: 0 10px 8px
  overflow: hidden

.tb-node__content
  font-size: 13px
  line-height: 1.5
  color: var(--color-text-primary)
  padding: 4px 8px
  border-radius: 4px

.tb-node__content--thinking
  color: var(--color-text-secondary)
  font-style: italic

.tb-node__content--action
  background: color-mix(in srgb, var(--color-warning) 4%, transparent)

.tb-node__content--reply
  color: var(--color-text-primary)

.tb-node__content--end
  color: var(--color-text-tertiary)
  font-style: italic

.tb-node__content--error
  color: var(--color-negative, var(--color-danger))

.tb-node__cursor
  display: inline-block
  width: 2px
  height: 14px
  background: var(--node-color)
  vertical-align: middle
  margin-left: 2px
  animation: tb-blink 0.8s step-end infinite

.tb-node__sub-board
  padding: 4px 0 0 8px

.tb-node__args, .tb-node__result
  margin-bottom: 6px

.tb-node__args-label, .tb-node__result-label
  font-size: 10px
  color: var(--color-text-tertiary)
  text-transform: uppercase
  letter-spacing: 0.3px
  margin-bottom: 3px

.tb-node__args pre, .tb-node__result pre
  margin: 0
  font-family: 'JetBrains Mono', 'Fira Code', monospace
  font-size: 11px
  line-height: 1.5
  color: var(--color-text-secondary)
  white-space: pre-wrap
  word-break: break-word
  max-height: 120px
  overflow-y: auto

// ── Collapse transition ──
.tb-collapse-enter-active,
.tb-collapse-leave-active
  transition: max-height 0.25s ease, opacity 0.15s ease
  overflow: hidden

.tb-collapse-enter-from,
.tb-collapse-leave-to
  max-height: 0
  opacity: 0

// ── Animations ──
@keyframes tb-fade-in
  from
    opacity: 0
    transform: translateY(3px)
  to
    opacity: 1
    transform: translateY(0)

@keyframes tb-pulse
  0%, 100%
    opacity: 1
  50%
    opacity: 0.3

@keyframes tb-blink
  0%, 100%
    opacity: 1
  50%
    opacity: 0
</style>
