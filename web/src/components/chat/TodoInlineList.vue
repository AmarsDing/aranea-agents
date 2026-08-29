<template>
  <div class="todo-inline-list" :class="`todo-inline-list--${effectiveStatus}`">
    <!-- Header: summary line -->
    <div class="todo-inline-list__header row items-center no-wrap q-gutter-xs">
      <q-icon name="checklist" :color="headerIconColor" size="18px" />
      <span class="text-weight-medium">{{ headerTitle }}</span>
      <span class="todo-inline-list__meta text-caption">{{ headerMeta }}</span>
      <q-space />
      <span v-if="isRunning" class="todo-inline-list__pulse" aria-hidden="true" />
      <span v-if="durationLabel" class="text-caption app-text-tertiary">{{
        durationLabel
      }}</span>
      <q-icon v-if="isFailed" name="error" color="negative" size="16px" />
      <q-icon v-else-if="isRunning" name="hourglass_top" color="warning" size="16px" />
      <q-icon v-else name="check_circle" color="positive" size="16px" />
    </div>

    <!-- Task cards -->
    <div class="todo-inline-list__cards">
      <TodoCard v-for="item in todoItems" :key="item.id" :item="item" />
    </div>

    <!-- Error message -->
    <div v-if="errorText" role="alert" class="todo-inline-list__error text-caption text-negative">
      {{ errorText }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ToolUseEvent } from '../../features/chat/types';
import type { TodoItem } from '../../features/chat/agentTreeTypes';
import { formatDurationLabel } from '../../features/chat/activityPresentation';
import { extractTodoItems } from '../../features/chat/todoPresentation';
import TodoCard from './TodoCard.vue';

const props = defineProps<{
  event: ToolUseEvent;
}>();

const { t } = useI18n();

// ── Data extraction ──

const todoItems = computed<TodoItem[]>(() => extractTodoItems(props.event));

// ── Status helpers ──

const isRunning = computed(() => props.event.status === 'running');
const isFailed = computed(() => props.event.status === 'failed' || props.event.status === 'error');
const effectiveStatus = computed(() => {
  if (isRunning.value) return 'running';
  if (isFailed.value) return 'failed';
  return 'completed';
});

// ── Header ──

const headerTitle = computed(() => t('chat.todo.taskPlan', '任务计划'));

const headerMeta = computed(() => {
  const total = todoItems.value.length;
  if (total === 0) return '';
  const inProgress = todoItems.value.filter((t) => t.status === 'in_progress').length;
  const completed = todoItems.value.filter((t) => t.status === 'completed').length;
  const pending = todoItems.value.filter((t) => t.status === 'pending').length;

  const parts: string[] = [];
  if (inProgress > 0) parts.push(t('chat.todo.inProgressCount', { count: inProgress }, '{count} 进行中'));
  if (pending > 0) parts.push(t('chat.todo.pendingCount', { count: pending }, '{count} 待处理'));
  if (completed > 0) parts.push(t('chat.todo.completedCount', { count: completed }, '{count} 已完成'));
  return `· ${total} ${t('chat.todo.items', '项')}` + (parts.length ? ` · ${parts.join(' · ')}` : '');
});

const headerIconColor = computed(() => {
  if (isFailed.value) return 'negative';
  if (isRunning.value) return 'warning';
  return 'positive';
});

const durationLabel = computed(() => formatDurationLabel(props.event.duration_ms));

const errorText = computed(() => {
  if (!isFailed.value) return '';
  return props.event.error?.trim() || props.event.error_code || '';
});
</script>

<style scoped lang="sass">
.todo-inline-list
  border-radius: 12px
  border: 1px solid color-mix(in srgb, var(--glass-border) 65%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  overflow: hidden
  transition: border-color 0.25s ease, box-shadow 0.25s ease

.todo-inline-list--running
  border-color: color-mix(in srgb, var(--color-warning) 30%, transparent)
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-warning) 8%, transparent)

.todo-inline-list--failed
  border-color: color-mix(in srgb, var(--color-danger) 30%, transparent)
  background: color-mix(in srgb, var(--color-danger) 4%, transparent)
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-danger) 8%, transparent)

.todo-inline-list--completed
  border-color: color-mix(in srgb, var(--color-success) 20%, transparent)
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-success) 6%, transparent)

.todo-inline-list__header
  padding: 10px 14px
  border-bottom: 1px solid color-mix(in srgb, var(--glass-border) 40%, transparent)

.todo-inline-list__meta
  color: var(--color-text-secondary)

.todo-inline-list__pulse
  display: inline-block
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-accent)
  animation: todo-pulse 1.4s ease-in-out infinite

.todo-inline-list__cards
  padding: 8px 10px 10px

.todo-inline-list__error
  padding: 6px 14px 10px

@keyframes todo-pulse
  0%, 100%
    opacity: 1
    transform: scale(1)
  50%
    opacity: 0.3
    transform: scale(0.85)
</style>
