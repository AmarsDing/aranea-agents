<template>
  <div class="tool-detail">
    <div v-if="total > 0" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.progress') }}</div>
      <div class="tool-detail__progress">
        <div class="tool-detail__progress-bar" :style="{ width: progressPercent + '%' }" />
      </div>
      <span class="tool-detail__progress-text">{{ completed }} / {{ total }}</span>
    </div>
    <div v-if="tasks.length" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.taskList') }}</div>
      <ul class="tool-detail__list">
        <li
          v-for="(task, idx) in tasks"
          :key="idx"
          class="tool-detail__task"
          :class="`tool-detail__task--${task.status}`"
        >
          <span class="tool-detail__task-icon">{{ statusIcon(task.status) }}</span>
          <span class="tool-detail__task-text">{{ task.content }}</span>
        </li>
      </ul>
    </div>
    <div v-if="activity.tool.error" class="tool-detail__row tool-detail__row--error">
      <div class="tool-detail__label">{{ t('chat.toolDetail.error') }}</div>
      <pre class="tool-detail__code">{{ activity.tool.error }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ActionEvent } from '../../../features/chat/streamEventTypes';
import { tryParseJson, asRecord, asArray, asString } from './toolDetailShared';

const { t } = useI18n();

type TaskStatus = 'pending' | 'in_progress' | 'completed' | 'cancelled';

interface TodoTask {
  content: string;
  status: TaskStatus;
}

const props = defineProps<{ activity: ActionEvent }>();

const parsedArgs = computed(() => asRecord(tryParseJson(props.activity.tool.arguments)));
const parsedResult = computed(() => asRecord(tryParseJson(props.activity.tool.result)));

const tasks = computed<TodoTask[]>(() => {
  // todo_write passes todos in arguments; some tools return them in result.
  const arr = asArray(parsedArgs.value?.todos ?? parsedResult.value?.todos ?? parsedResult.value?.tasks);
  if (!arr) return [];
  return arr
    .map((item): TodoTask | undefined => {
      const rec = asRecord(item);
      if (!rec) return undefined;
      const content = asString(rec.content) ?? asString(rec.task) ?? asString(rec.text) ?? '';
      const rawStatus = asString(rec.status) ?? 'pending';
      const status: TaskStatus = normalizeStatus(rawStatus);
      if (!content) return undefined;
      return { content, status };
    })
    .filter((x): x is TodoTask => x !== undefined);
});

const total = computed(() => tasks.value.length);
const completed = computed(() => tasks.value.filter((t) => t.status === 'completed').length);
const progressPercent = computed(() => {
  if (total.value === 0) return 0;
  return Math.round((completed.value / total.value) * 100);
});

function normalizeStatus(raw: string): TaskStatus {
  const lower = raw.toLowerCase();
  if (lower === 'completed' || lower === 'done') return 'completed';
  if (lower === 'in_progress' || lower === 'in-progress' || lower === 'running') return 'in_progress';
  if (lower === 'cancelled' || lower === 'skipped') return 'cancelled';
  return 'pending';
}

function statusIcon(status: TaskStatus): string {
  switch (status) {
    case 'completed':
      return '✓';
    case 'in_progress':
      return '▸';
    case 'cancelled':
      return '⊘';
    default:
      return '○';
  }
}
</script>

<style lang="sass" scoped>
.tool-detail
  &__row
    margin-bottom: 6px
    &--error
      .tool-detail__code
        border-color: var(--color-danger)

  &__label
    font-size: 11px
    color: var(--color-text-secondary)
    margin-bottom: 4px

  &__progress
    height: 6px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 3px
    overflow: hidden
    margin-bottom: 4px

  &__progress-bar
    height: 100%
    background: var(--color-accent)
    transition: width 0.2s ease

  &__progress-text
    font-size: 12px
    color: var(--color-text-secondary)

  &__list
    list-style: none
    padding: 0
    margin: 0
    max-height: 240px
    overflow-y: auto

  &__task
    display: flex
    align-items: flex-start
    gap: 6px
    padding: 3px 0
    font-size: 12px
    color: var(--color-text-primary)
    border-bottom: 1px solid var(--glass-border)
    &:last-child
      border-bottom: none

  &__task-icon
    flex-shrink: 0
    width: 14px
    text-align: center
    color: var(--color-text-secondary)

  &__task--completed
    .tool-detail__task-text
      text-decoration: line-through
      color: var(--color-text-secondary)

  &__task--in_progress
    .tool-detail__task-icon
      color: var(--color-accent)

  &__task--cancelled
    .tool-detail__task-text
      color: var(--color-text-secondary)
      text-decoration: line-through

  &__code
    font-size: 12px
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: 6px 8px
    overflow-x: auto
    max-height: 240px
    overflow-y: auto
    margin: 0
    white-space: pre-wrap
    word-break: break-word
</style>
